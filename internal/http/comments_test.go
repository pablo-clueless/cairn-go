package http_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestCommentsCRUDAndAuthorOnly(t *testing.T) {
	srv := newTestServer(t)
	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	slug := createOrg(t, alice, "Acme Inc")

	resp, body := alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "ENG", "name": "Engineering"})
	mustStatus(t, resp, body, http.StatusCreated)
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/issues", map[string]string{"title": "A"})
	mustStatus(t, resp, body, http.StatusCreated)
	key := jsonField(t, body, "key")

	// Empty body is rejected.
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/issues/"+key+"/comments", map[string]string{"body": "   "})
	mustStatus(t, resp, body, http.StatusBadRequest)

	// Create a comment.
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/issues/"+key+"/comments", map[string]string{"body": "first!"})
	mustStatus(t, resp, body, http.StatusCreated)
	cid := jsonField(t, body, "id")

	// List -> 1, with the author name resolved.
	resp, body = alice.do("GET", "/v1/orgs/"+slug+"/issues/"+key+"/comments", nil)
	mustStatus(t, resp, body, http.StatusOK)
	var comments []struct {
		ID         string `json:"id"`
		Body       string `json:"body"`
		AuthorName string `json:"author_name"`
	}
	decodeData(t, body, &comments)
	if len(comments) != 1 || comments[0].AuthorName != "Alice" {
		t.Fatalf("unexpected comments: %+v", comments)
	}

	// Author can edit.
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/comments/"+cid, map[string]string{"body": "edited"})
	mustStatus(t, resp, body, http.StatusOK)
	if b := jsonField(t, body, "body"); b != "edited" {
		t.Fatalf("expected edited, got %s", b)
	}

	// Bob joins as a member but cannot edit Alice's comment -> 403.
	bob := newClient(t, srv.URL)
	bob.signupUser("bob@example.com", "Bob", "supersecret123")
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/invitations", map[string]string{"email": "bob@example.com", "role": "member"})
	mustStatus(t, resp, body, http.StatusCreated)
	var inv struct {
		AcceptURL string `json:"accept_url"`
	}
	decodeData(t, body, &inv)
	token := strings.Split(inv.AcceptURL, "token=")[1]
	resp, body = bob.do("PATCH", "/v1/invitations/"+token, map[string]string{"status": "accepted"})
	mustStatus(t, resp, body, http.StatusOK)
	resp, body = bob.do("PATCH", "/v1/orgs/"+slug+"/comments/"+cid, map[string]string{"body": "hijack"})
	mustStatus(t, resp, body, http.StatusForbidden)

	// Author deletes; list is empty again.
	resp, body = alice.do("DELETE", "/v1/orgs/"+slug+"/comments/"+cid, nil)
	mustStatus(t, resp, body, http.StatusNoContent)
	resp, body = alice.do("GET", "/v1/orgs/"+slug+"/issues/"+key+"/comments", nil)
	mustStatus(t, resp, body, http.StatusOK)
	decodeData(t, body, &comments)
	if len(comments) != 0 {
		t.Fatalf("expected 0 comments after delete, got %d", len(comments))
	}
}
