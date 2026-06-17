package http_test

import (
	"net/http"
	"testing"
)

func TestSprintLifecycle(t *testing.T) {
	srv := newTestServer(t)
	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	slug := createOrg(t, alice, "Acme Inc")

	// Space + two issues.
	resp, body := alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "ENG", "name": "Engineering"})
	mustStatus(t, resp, body, http.StatusCreated)
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/issues", map[string]string{"title": "A"})
	mustStatus(t, resp, body, http.StatusCreated)
	keyA := jsonField(t, body, "key")
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/issues", map[string]string{"title": "B"})
	mustStatus(t, resp, body, http.StatusCreated)
	keyB := jsonField(t, body, "key")

	// Create a sprint (planned).
	resp, body = alice.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/sprints", map[string]string{"name": "Sprint 1"})
	mustStatus(t, resp, body, http.StatusCreated)
	sprintID := jsonField(t, body, "id")
	if s := jsonField(t, body, "status"); s != "planned" {
		t.Fatalf("expected planned, got %s", s)
	}

	// Assign both issues to the sprint.
	for _, k := range []string{keyA, keyB} {
		resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/issues/"+k, map[string]string{"sprint_id": sprintID})
		mustStatus(t, resp, body, http.StatusOK)
	}

	// Sprint has 2 issues; backlog has 0.
	resp, body = alice.do("GET", "/v1/orgs/"+slug+"/issues?sprint="+sprintID, nil)
	mustStatus(t, resp, body, http.StatusOK)
	var inSprint []map[string]any
	decodeData(t, body, &inSprint)
	if len(inSprint) != 2 {
		t.Fatalf("expected 2 in sprint, got %d", len(inSprint))
	}

	// Complete one issue, then start + complete the sprint.
	doneID := statusIDByCategory(t, alice, slug, "ENG", "done")
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/issues/"+keyA, map[string]string{"status_id": doneID})
	mustStatus(t, resp, body, http.StatusOK)
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/sprints/"+sprintID, map[string]string{"status": "active"})
	mustStatus(t, resp, body, http.StatusOK)
	if s := jsonField(t, body, "status"); s != "active" {
		t.Fatalf("expected active, got %s", s)
	}
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/sprints/"+sprintID, map[string]string{"status": "completed"})
	mustStatus(t, resp, body, http.StatusOK)

	// Incomplete issue (B) moved to backlog; done issue (A) stays in sprint.
	resp, body = alice.do("GET", "/v1/orgs/"+slug+"/issues/"+keyB, nil)
	mustStatus(t, resp, body, http.StatusOK)
	var issueB struct {
		SprintID *string `json:"sprint_id"`
	}
	decodeData(t, body, &issueB)
	if issueB.SprintID != nil {
		t.Fatalf("expected B back in backlog, got sprint %v", *issueB.SprintID)
	}
	resp, body = alice.do("GET", "/v1/orgs/"+slug+"/issues?sprint="+sprintID, nil)
	mustStatus(t, resp, body, http.StatusOK)
	decodeData(t, body, &inSprint)
	if len(inSprint) != 1 {
		t.Fatalf("expected 1 (done) issue kept in sprint, got %d", len(inSprint))
	}
}

func TestOnlyOneActiveSprint(t *testing.T) {
	srv := newTestServer(t)
	alice := newClient(t, srv.URL)
	alice.signupUser("alice@example.com", "Alice", "supersecret123")
	slug := createOrg(t, alice, "Acme Inc")

	resp, body := alice.do("POST", "/v1/orgs/"+slug+"/spaces", map[string]string{"key": "ENG", "name": "Engineering"})
	mustStatus(t, resp, body, http.StatusCreated)

	mkSprint := func(name string) string {
		resp, body := alice.do("POST", "/v1/orgs/"+slug+"/spaces/ENG/sprints", map[string]string{"name": name})
		mustStatus(t, resp, body, http.StatusCreated)
		return jsonField(t, body, "id")
	}
	a := mkSprint("A")
	b := mkSprint("B")

	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/sprints/"+a, map[string]string{"status": "active"})
	mustStatus(t, resp, body, http.StatusOK)

	// Starting a second sprint while one is active -> 409.
	resp, body = alice.do("PATCH", "/v1/orgs/"+slug+"/sprints/"+b, map[string]string{"status": "active"})
	mustStatus(t, resp, body, http.StatusConflict)
}
