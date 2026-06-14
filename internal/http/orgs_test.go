package http_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestOrgLifecycleAndTenancy(t *testing.T) {
	srv := newTestServer(t)

	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	bob := newClient(t, srv.URL)
	bob.signupUser("bob@example.com", "Bob", "supersecret123")

	// Alice creates an org.
	resp, body := alice.do("POST", "/v1/orgs", map[string]string{"name": "Acme Inc"})
	mustStatus(t, resp, body, http.StatusCreated)
	var org struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	}
	decodeData(t, body, &org)
	if org.ID == "" || org.Slug == "" {
		t.Fatalf("expected id and slug, got %s", body)
	}

	// Alice lists her orgs -> 1.
	resp, body = alice.do("GET", "/v1/orgs", nil)
	mustStatus(t, resp, body, http.StatusOK)
	var orgs []map[string]any
	decodeData(t, body, &orgs)
	if len(orgs) != 1 {
		t.Fatalf("expected 1 org, got %d", len(orgs))
	}

	// Bob (non-member) cannot see the org -> 404 (no existence leak).
	resp, body = bob.do("GET", "/v1/orgs/"+org.ID, nil)
	mustStatus(t, resp, body, http.StatusNotFound)

	// Alice sees the org with her role.
	resp, body = alice.do("GET", "/v1/orgs/"+org.ID, nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !strings.Contains(string(body), `"role":"owner"`) {
		t.Fatalf("expected owner role, got %s", body)
	}
}

func TestInviteAcceptAndRBAC(t *testing.T) {
	srv := newTestServer(t)

	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	bob := newClient(t, srv.URL)
	bob.signupUser("bob@example.com", "Bob", "supersecret123")

	// Alice creates an org.
	resp, body := alice.do("POST", "/v1/orgs", map[string]string{"name": "Acme Inc"})
	mustStatus(t, resp, body, http.StatusCreated)
	orgID := jsonField(t, body, "id")

	// Alice invites Bob; capture the accept token from the returned URL.
	resp, body = alice.do("POST", "/v1/orgs/"+orgID+"/invitations", map[string]string{
		"email": "bob@example.com", "role": "member",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	var inv struct {
		AcceptURL string `json:"accept_url"`
	}
	decodeData(t, body, &inv)
	parts := strings.Split(inv.AcceptURL, "token=")
	if len(parts) != 2 || parts[1] == "" {
		t.Fatalf("could not extract token from %q", inv.AcceptURL)
	}
	token := parts[1]

	// Duplicate invite -> 409.
	resp, body = alice.do("POST", "/v1/orgs/"+orgID+"/invitations", map[string]string{
		"email": "bob@example.com", "role": "member",
	})
	mustStatus(t, resp, body, http.StatusConflict)

	// Bob accepts -> 200, joins org.
	resp, body = bob.do("POST", "/v1/invitations/accept", map[string]string{"token": token})
	mustStatus(t, resp, body, http.StatusOK)

	// Members now number 2.
	resp, body = alice.do("GET", "/v1/orgs/"+orgID+"/members", nil)
	mustStatus(t, resp, body, http.StatusOK)
	var members []struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
		Role   string `json:"role"`
	}
	decodeData(t, body, &members)
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d (%s)", len(members), body)
	}

	// Bob (member) cannot invite -> 403.
	resp, body = bob.do("POST", "/v1/orgs/"+orgID+"/invitations", map[string]string{
		"email": "carol@example.com", "role": "member",
	})
	mustStatus(t, resp, body, http.StatusForbidden)

	// Find ids.
	var bobID, aliceID string
	for _, m := range members {
		switch m.Email {
		case "bob@example.com":
			bobID = m.UserID
		case "alice@example.com":
			aliceID = m.UserID
		}
	}

	// Alice promotes Bob to admin -> 204.
	resp, body = alice.do("PATCH", "/v1/orgs/"+orgID+"/members/"+bobID, map[string]string{"role": "admin"})
	mustStatus(t, resp, body, http.StatusNoContent)

	// Removing the last owner (Alice) is blocked -> 400.
	resp, body = alice.do("DELETE", "/v1/orgs/"+orgID+"/members/"+aliceID, nil)
	mustStatus(t, resp, body, http.StatusBadRequest)
}

func TestAcceptInviteEmailMismatch(t *testing.T) {
	srv := newTestServer(t)

	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	mallory := newClient(t, srv.URL)
	mallory.signupUser("mallory@example.com", "Mallory", "supersecret123")

	resp, body := alice.do("POST", "/v1/orgs", map[string]string{"name": "Acme Inc"})
	mustStatus(t, resp, body, http.StatusCreated)
	orgID := jsonField(t, body, "id")

	resp, body = alice.do("POST", "/v1/orgs/"+orgID+"/invitations", map[string]string{
		"email": "bob@example.com", "role": "member",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	var inv struct {
		AcceptURL string `json:"accept_url"`
	}
	decodeData(t, body, &inv)
	token := strings.Split(inv.AcceptURL, "token=")[1]

	// Mallory holds the link but it was issued to bob@ -> 403.
	resp, body = mallory.do("POST", "/v1/invitations/accept", map[string]string{"token": token})
	mustStatus(t, resp, body, http.StatusForbidden)
}

// jsonField reads a string field from the "data" object of a success envelope.
func jsonField(t *testing.T, body []byte, field string) string {
	t.Helper()
	var m map[string]any
	decodeData(t, body, &m)
	v, ok := m[field].(string)
	if !ok {
		t.Fatalf("field %q not found in %s", field, body)
	}
	return v
}
