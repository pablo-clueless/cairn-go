package http_test

import (
	"net/http"
	"testing"
)

func TestAuthFlow(t *testing.T) {
	srv := newTestServer(t)
	c := newClient(t, srv.URL)
	const email = "alice@example.com"

	// Signup -> 201 with access token + refresh cookie.
	resp, body := c.do("POST", "/api/v1/auth/signup", map[string]string{
		"email": email, "name": "Alice", "password": "supersecret123",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	c.token = accessTokenFrom(t, body)

	// Authenticated /me -> 200.
	resp, body = c.do("GET", "/api/v1/auth/me", nil)
	mustStatus(t, resp, body, http.StatusOK)

	// Unauthenticated /me -> 401.
	anon := newClient(t, srv.URL)
	resp, body = anon.do("GET", "/api/v1/auth/me", nil)
	mustStatus(t, resp, body, http.StatusUnauthorized)

	// Refresh via cookie -> 200 with a new access token.
	c.token = ""
	resp, body = c.do("POST", "/api/v1/auth/refresh", nil)
	mustStatus(t, resp, body, http.StatusOK)
	c.token = accessTokenFrom(t, body)

	// Login -> 200.
	resp, body = c.do("POST", "/api/v1/auth/login", map[string]string{
		"email": email, "password": "supersecret123",
	})
	mustStatus(t, resp, body, http.StatusOK)

	// Wrong password -> 401.
	resp, body = c.do("POST", "/api/v1/auth/login", map[string]string{
		"email": email, "password": "wrong",
	})
	mustStatus(t, resp, body, http.StatusUnauthorized)

	// Duplicate signup -> 409.
	resp, body = c.do("POST", "/api/v1/auth/signup", map[string]string{
		"email": email, "name": "Alice", "password": "supersecret123",
	})
	mustStatus(t, resp, body, http.StatusConflict)
}

func TestSignupValidation(t *testing.T) {
	srv := newTestServer(t)
	c := newClient(t, srv.URL)

	cases := []map[string]string{
		{"email": "bad", "name": "X", "password": "supersecret123"}, // invalid email
		{"email": "x@y.com", "name": "", "password": "supersecret123"}, // missing name
		{"email": "x@y.com", "name": "X", "password": "short"},        // weak password
	}
	for _, payload := range cases {
		resp, body := c.do("POST", "/api/v1/auth/signup", payload)
		mustStatus(t, resp, body, http.StatusBadRequest)
	}
}

func TestOAuthDisabledReturns404(t *testing.T) {
	srv := newTestServer(t)
	c := newClient(t, srv.URL)
	// Test config has no provider credentials, so SSO is disabled.
	resp, body := c.do("GET", "/api/v1/auth/oauth/google", nil)
	mustStatus(t, resp, body, http.StatusNotFound)
}
