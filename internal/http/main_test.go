package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cairn/internal/config"
	httpapi "cairn/internal/http"
	"cairn/internal/store"
	"cairn/migrations"
)

// testDatabaseURL returns the URL used for integration tests, or "" to skip.
func testDatabaseURL() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return os.Getenv("DATABASE_URL")
}

func testConfig() config.Config {
	return config.Config{
		Env:             "test",
		Port:            "0",
		CORSOrigin:      "http://localhost:3001",
		FrontendURL:     "http://localhost:3001",
		AppBaseURL:      "http://localhost:8000",
		JWTSecret:       "test-secret-key-for-integration-tests-0123456789",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
		InviteTTL:       time.Hour,
		CookieSecure:    false,
	}
}

// newTestServer spins up the full HTTP stack and returns just the server.
func newTestServer(t *testing.T) *httptest.Server {
	srv, _ := newTestEnv(t)
	return srv
}

// newTestEnv spins up the full HTTP stack against an isolated Postgres schema
// (dropped on cleanup) and returns the server plus the store for direct setup.
// Tests are skipped when no DB is set.
func newTestEnv(t *testing.T) (*httptest.Server, *store.DB) {
	t.Helper()

	url := testDatabaseURL()
	if url == "" {
		t.Skip("set TEST_DATABASE_URL (or DATABASE_URL) to run integration tests")
	}

	ctx := context.Background()
	schema := fmt.Sprintf("cairn_test_%d", time.Now().UnixNano())

	// Create the isolated schema with a one-off connection.
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close(ctx)
		t.Fatalf("create schema: %v", err)
	}
	admin.Close(ctx)

	// Pool pinned to the isolated schema.
	poolCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	poolCfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	db := &store.DB{Pool: pool}
	if err := db.Migrate(ctx, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	srv := httptest.NewServer(httpapi.NewServer(db, testConfig()).Router())

	t.Cleanup(func() {
		srv.Close()
		pool.Close()
		if a, err := pgx.Connect(ctx, url); err == nil {
			_, _ = a.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
			a.Close(ctx)
		}
	})
	return srv, db
}

// apiClient is a cookie-aware test HTTP client with a bearer token.
type apiClient struct {
	t     *testing.T
	base  string
	hc    *http.Client
	token string
}

func newClient(t *testing.T, base string) *apiClient {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &apiClient{t: t, base: base, hc: &http.Client{Jar: jar}}
}

// do issues a request and returns the response plus its body bytes.
func (c *apiClient) do(method, path string, body any) (*http.Response, []byte) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatalf("do request: %v", err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, data
}

// signupUser registers a user and stores the returned access token on the client.
func (c *apiClient) signupUser(email, name, password string) {
	c.t.Helper()
	resp, body := c.do("POST", "/v1/auth/signup", map[string]string{
		"email": email, "name": name, "password": password,
	})
	if resp.StatusCode != http.StatusCreated {
		c.t.Fatalf("signup %s: status %d body %s", email, resp.StatusCode, body)
	}
	c.token = accessTokenFrom(c.t, body)
}

func accessTokenFrom(t *testing.T, body []byte) string {
	t.Helper()
	var out struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode access token: %v (body=%s)", err, body)
	}
	if out.Data.AccessToken == "" {
		t.Fatalf("empty access token (body=%s)", body)
	}
	return out.Data.AccessToken
}

// decodeData unmarshals the "data" field of a success envelope into dst.
func decodeData(t *testing.T, body []byte, dst any) {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, body)
	}
	if err := json.Unmarshal(env.Data, dst); err != nil {
		t.Fatalf("decode data: %v (body=%s)", err, body)
	}
}

func mustStatus(t *testing.T, resp *http.Response, body []byte, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d (body=%s)", resp.StatusCode, want, body)
	}
}
