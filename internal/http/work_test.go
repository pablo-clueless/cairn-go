package http_test

import (
	"net/http"
	"strings"
	"testing"
)

// createOrg signs the client up (owner) and returns the org slug.
func createOrg(t *testing.T, c *apiClient, name string) string {
	t.Helper()
	resp, body := c.do("POST", "/v1/orgs", map[string]string{"name": name})
	mustStatus(t, resp, body, http.StatusCreated)
	return jsonField(t, body, "slug")
}

func TestSpaceAndIssueLifecycle(t *testing.T) {
	srv := newTestServer(t)
	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	slug := createOrg(t, alice, "Acme Inc")

	// Create a space.
	resp, body := alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "eng", "name": "Engineering"})
	mustStatus(t, resp, body, http.StatusCreated)
	var space struct {
		Key        string `json:"key"`
		IssueCount int    `json:"issue_count"`
	}
	decodeData(t, body, &space)
	if space.Key != "ENG" { // normalized to uppercase
		t.Fatalf("expected key ENG, got %s", space.Key)
	}

	// Create an issue -> ENG-1.
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/issues", map[string]string{
		"type": "story", "title": "Set up CI", "priority": "high",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	var issue struct {
		Key            string `json:"key"`
		StatusCategory string `json:"status_category"`
		Number         int    `json:"number"`
	}
	decodeData(t, body, &issue)
	if issue.Key != "ENG-1" || issue.Number != 1 || issue.StatusCategory != "todo" {
		t.Fatalf("unexpected issue: %+v", issue)
	}

	// Second issue -> ENG-2 (sequence increments).
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/issues", map[string]string{
		"type": "bug", "title": "Fix redirect",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	if k := jsonFieldFromData(t, body, "key"); k != "ENG-2" {
		t.Fatalf("expected ENG-2, got %s", k)
	}

	// Update status via PATCH (status is now a per-space status id).
	inProgress := statusIDByCategory(t, alice, slug, "ENG", "in_progress")
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/issues/ENG-1", map[string]string{"status_id": inProgress})
	mustStatus(t, resp, body, http.StatusOK)
	if s := jsonFieldFromData(t, body, "status_category"); s != "in_progress" {
		t.Fatalf("expected in_progress, got %s", s)
	}

	// Get by key.
	resp, body = alice.do("GET", "/v1/orgs/"+slug+"/issues/ENG-2", nil)
	mustStatus(t, resp, body, http.StatusOK)

	// List issues -> 2.
	resp, body = alice.do("GET", "/v1/orgs/"+slug+"/issues", nil)
	mustStatus(t, resp, body, http.StatusOK)
	var issues []map[string]any
	decodeData(t, body, &issues)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	// Delete an issue.
	resp, body = alice.do("DELETE", "/v1/orgs/"+slug+"/issues/ENG-2", nil)
	mustStatus(t, resp, body, http.StatusNoContent)
	resp, body = alice.do("GET", "/v1/orgs/"+slug+"/issues/ENG-2", nil)
	mustStatus(t, resp, body, http.StatusNotFound)
}

func TestSpaceValidationAndConflict(t *testing.T) {
	srv := newTestServer(t)
	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	slug := createOrg(t, alice, "Acme Inc")

	// Invalid key.
	resp, body := alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "bad-key", "name": "X"})
	mustStatus(t, resp, body, http.StatusBadRequest)

	// Valid, then duplicate.
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "ENG", "name": "Engineering"})
	mustStatus(t, resp, body, http.StatusCreated)
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "ENG", "name": "Dup"})
	mustStatus(t, resp, body, http.StatusConflict)

	// Unknown issue key -> 404.
	resp, body = alice.do("GET", "/v1/orgs/"+slug+"/issues/ENG-999", nil)
	mustStatus(t, resp, body, http.StatusNotFound)
}

func TestSpaceRBAC(t *testing.T) {
	srv := newTestServer(t)
	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	bob := newClient(t, srv.URL)
	bob.signupUser("bob@example.com", "Bob", "supersecret123")

	slug := createOrg(t, alice, "Acme Inc")

	// Invite Bob as a member and have him accept.
	resp, body := alice.do("POST", "/v1/orgs/"+slug+"/invitations", map[string]string{"email": "bob@example.com", "role": "member"})
	mustStatus(t, resp, body, http.StatusCreated)
	var inv struct {
		AcceptURL string `json:"accept_url"`
	}
	decodeData(t, body, &inv)
	token := strings.Split(inv.AcceptURL, "token=")[1]
	resp, body = bob.do("PATCH", "/v1/invitations/"+token, map[string]string{"status": "accepted"})
	mustStatus(t, resp, body, http.StatusOK)

	// Alice (owner) creates a space.
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "ENG", "name": "Engineering"})
	mustStatus(t, resp, body, http.StatusCreated)

	// Bob (member) cannot create a space -> 403.
	resp, body = bob.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "OPS", "name": "Ops"})
	mustStatus(t, resp, body, http.StatusForbidden)

	// But Bob (member) can create an issue -> 201.
	resp, body = bob.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/issues", map[string]string{"title": "Member task"})
	mustStatus(t, resp, body, http.StatusCreated)
}

// jsonFieldFromData reads a string field from a success envelope's data object.
func jsonFieldFromData(t *testing.T, body []byte, field string) string {
	t.Helper()
	return jsonField(t, body, field)
}

// statusIDByCategory returns the id of a space's status with the given category.
func statusIDByCategory(t *testing.T, c *apiClient, slug, spaceKey, category string) string {
	t.Helper()
	resp, body := c.do("GET", "/v1/orgs/"+slug+"/spaces/"+spaceKey+"/statuses", nil)
	mustStatus(t, resp, body, http.StatusOK)
	var statuses []struct {
		ID       string `json:"id"`
		Category string `json:"category"`
	}
	decodeData(t, body, &statuses)
	for _, s := range statuses {
		if s.Category == category {
			return s.ID
		}
	}
	t.Fatalf("no status with category %q for space %s", category, spaceKey)
	return ""
}
