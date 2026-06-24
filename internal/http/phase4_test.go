package http_test

import (
	"net/http"
	"testing"
)

// TestIssueTransitionEnforcement verifies the per-space workflow is enforced
// server-side: an open workflow (no transitions configured) allows any change,
// a configured workflow rejects edges that aren't listed, and clearing the
// workflow reopens it.
func TestIssueTransitionEnforcement(t *testing.T) {
	srv := newTestServer(t)
	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	slug := createOrg(t, alice, "Acme Inc")

	resp, body := alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "ENG", "name": "Engineering"})
	mustStatus(t, resp, body, http.StatusCreated)

	todo := statusIDByCategory(t, alice, slug, "ENG", "todo")
	inProgress := statusIDByCategory(t, alice, slug, "ENG", "in_progress")
	done := statusIDByCategory(t, alice, slug, "ENG", "done")

	mkIssue := func() string {
		resp, body := alice.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/issues", map[string]string{"title": "T"})
		mustStatus(t, resp, body, http.StatusCreated)
		return jsonField(t, body, "key")
	}

	// Open workflow: jumping todo -> done is allowed.
	open := mkIssue()
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/issues/"+open, map[string]string{"status_id": done})
	mustStatus(t, resp, body, http.StatusOK)

	// Configure a linear workflow: todo -> in_progress -> done.
	resp, body = alice.do("PUT", "/v1/orgs/"+slug+"/spaces/ENG/transitions", map[string]any{
		"transitions": []map[string]any{
			{"from_status_id": todo, "to_status_id": inProgress},
			{"from_status_id": inProgress, "to_status_id": done},
		},
	})
	mustStatus(t, resp, body, http.StatusOK)

	// A fresh issue starts in todo. Skipping straight to done is now rejected.
	issue := mkIssue()
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/issues/"+issue, map[string]string{"status_id": done})
	mustStatus(t, resp, body, http.StatusConflict)

	// The configured edges are allowed, in order.
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/issues/"+issue, map[string]string{"status_id": inProgress})
	mustStatus(t, resp, body, http.StatusOK)
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/issues/"+issue, map[string]string{"status_id": done})
	mustStatus(t, resp, body, http.StatusOK)

	// Clearing the workflow reopens it: todo -> done allowed again.
	resp, body = alice.do("PUT", "/v1/orgs/"+slug+"/spaces/ENG/transitions", map[string]any{
		"transitions": []map[string]any{},
	})
	mustStatus(t, resp, body, http.StatusOK)
	reopened := mkIssue()
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/issues/"+reopened, map[string]string{"status_id": done})
	mustStatus(t, resp, body, http.StatusOK)
}

// TestSprintInvalidStatusTransition verifies the sprint state machine only
// permits planned -> active -> completed.
func TestSprintInvalidStatusTransition(t *testing.T) {
	srv := newTestServer(t)
	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	slug := createOrg(t, alice, "Acme Inc")

	resp, body := alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "ENG", "name": "Engineering"})
	mustStatus(t, resp, body, http.StatusCreated)

	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/sprints", map[string]string{"name": "S1"})
	mustStatus(t, resp, body, http.StatusCreated)
	id := jsonField(t, body, "id")

	// planned -> completed (skipping active) is rejected.
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/sprints/"+id, map[string]string{"status": "completed"})
	mustStatus(t, resp, body, http.StatusConflict)

	// planned -> active is allowed.
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/sprints/"+id, map[string]string{"status": "active"})
	mustStatus(t, resp, body, http.StatusOK)

	// active -> planned (backwards) is rejected.
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/sprints/"+id, map[string]string{"status": "planned"})
	mustStatus(t, resp, body, http.StatusConflict)
}

// TestStatusManagementAndWIPLimit verifies status create/rename/recolor, that
// the per-column WIP limit (migration 0013) round-trips, and delete.
func TestStatusManagementAndWIPLimit(t *testing.T) {
	srv := newTestServer(t)
	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	slug := createOrg(t, alice, "Acme Inc")

	resp, body := alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "ENG", "name": "Engineering"})
	mustStatus(t, resp, body, http.StatusCreated)

	// Create a custom status.
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/statuses", map[string]string{
		"name": "Review", "category": "in_progress", "color": "#abcabc",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	statusID := jsonField(t, body, "id")

	// Rename, recolor, and set a WIP limit.
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/statuses/"+statusID, map[string]any{
		"name": "In Review", "color": "#123123", "wip_limit": 3,
	})
	mustStatus(t, resp, body, http.StatusOK)

	// Changes persist (verified via the space's status list).
	resp, body = alice.do("GET", "/v1/orgs/"+slug+"/spaces/ENG/statuses", nil)
	mustStatus(t, resp, body, http.StatusOK)
	var statuses []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Color    string `json:"color"`
		WIPLimit int    `json:"wip_limit"`
	}
	decodeData(t, body, &statuses)
	var found bool
	for _, st := range statuses {
		if st.ID == statusID {
			found = true
			if st.Name != "In Review" || st.Color != "#123123" || st.WIPLimit != 3 {
				t.Fatalf("status changes not persisted: %+v", st)
			}
		}
	}
	if !found {
		t.Fatalf("custom status missing from list (body=%s)", body)
	}

	// Delete the custom status (no issues use it).
	resp, body = alice.do("DELETE", "/v1/orgs/"+slug+"/statuses/"+statusID, nil)
	mustStatus(t, resp, body, http.StatusNoContent)
}

// TestIssueRankPersists verifies the fractional sort key (lexorank, migration
// 0012) survives a round-trip — the backbone of stable board/backlog ordering.
func TestIssueRankPersists(t *testing.T) {
	srv := newTestServer(t)
	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	slug := createOrg(t, alice, "Acme Inc")

	resp, body := alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "ENG", "name": "Engineering"})
	mustStatus(t, resp, body, http.StatusCreated)
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/issues", map[string]string{"title": "A"})
	mustStatus(t, resp, body, http.StatusCreated)
	key := jsonField(t, body, "key")

	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/issues/"+key, map[string]any{"rank": 2.5})
	mustStatus(t, resp, body, http.StatusOK)

	resp, body = alice.do("GET", "/v1/orgs/"+slug+"/issues/"+key, nil)
	mustStatus(t, resp, body, http.StatusOK)
	var issue struct {
		Rank float64 `json:"rank"`
	}
	decodeData(t, body, &issue)
	if issue.Rank != 2.5 {
		t.Fatalf("rank not persisted: got %v, want 2.5", issue.Rank)
	}
}
