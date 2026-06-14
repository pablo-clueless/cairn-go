package http_test

import (
	"context"
	"net/http"
	"testing"
)

func TestSubscriptionDefaultsInactive(t *testing.T) {
	srv := newTestServer(t)
	c := newClient(t, srv.URL)
	c.signupUser("alice@example.com", "Alice", "supersecret123")

	resp, body := c.do("POST", "/v1/orgs", map[string]string{"name": "Acme Inc"})
	mustStatus(t, resp, body, http.StatusCreated)
	orgID := jsonField(t, body, "id")

	// Non-admin org creation => inactive subscription, one seat (the owner).
	resp, body = c.do("GET", "/v1/orgs/"+orgID+"/subscription", nil)
	mustStatus(t, resp, body, http.StatusOK)
	var sub struct {
		BillingEnabled bool   `json:"billing_enabled"`
		Status         string `json:"status"`
		Seats          int    `json:"seats"`
	}
	decodeData(t, body, &sub)
	if sub.BillingEnabled || sub.Status != "inactive" || sub.Seats != 1 {
		t.Fatalf("expected inactive 1-seat sub, got %+v", sub)
	}
}

func TestNonAdminCannotAccessAdminRoutes(t *testing.T) {
	srv := newTestServer(t)
	c := newClient(t, srv.URL)
	c.signupUser("alice@example.com", "Alice", "supersecret123")

	resp, body := c.do("GET", "/v1/admin/settings", nil)
	mustStatus(t, resp, body, http.StatusForbidden)
}

func TestPlatformAdminBillingLifecycle(t *testing.T) {
	srv, db := newTestEnv(t)
	admin := newClient(t, srv.URL)
	admin.signupUser("ops@cairn.dev", "Ops", "supersecret123")

	// Grant platform admin directly (mirrors startup bootstrap). Middleware
	// reloads the user per request, so the existing token now has admin rights.
	if err := db.SetPlatformAdminByEmails(context.Background(), []string{"ops@cairn.dev"}); err != nil {
		t.Fatalf("grant admin: %v", err)
	}

	// /v1/me reflects platform-admin.
	resp, body := admin.do("GET", "/v1/me", nil)
	mustStatus(t, resp, body, http.StatusOK)
	var me struct {
		IsPlatformAdmin bool `json:"is_platform_admin"`
	}
	decodeData(t, body, &me)
	if !me.IsPlatformAdmin {
		t.Fatalf("expected platform admin, got %+v", me)
	}

	// Update global default trial days.
	resp, body = admin.do("PATCH", "/v1/admin/settings", map[string]int{"default_trial_days": 30})
	mustStatus(t, resp, body, http.StatusOK)
	var settings struct {
		DefaultTrialDays int `json:"default_trial_days"`
	}
	decodeData(t, body, &settings)
	if settings.DefaultTrialDays != 30 {
		t.Fatalf("expected 30 trial days, got %d", settings.DefaultTrialDays)
	}

	// Create an org, then enable billing => trialing with a future trial end.
	resp, body = admin.do("POST", "/v1/orgs", map[string]string{"name": "Acme Inc"})
	mustStatus(t, resp, body, http.StatusCreated)
	orgID := jsonField(t, body, "id")

	resp, body = admin.do("PATCH", "/v1/admin/orgs/"+orgID+"/subscription", map[string]any{
		"billing_enabled":      true,
		"price_per_seat_cents": 1500,
	})
	mustStatus(t, resp, body, http.StatusOK)
	var sub struct {
		BillingEnabled     bool   `json:"billing_enabled"`
		Status             string `json:"status"`
		Seats              int    `json:"seats"`
		PricePerSeatCents  int    `json:"price_per_seat_cents"`
		AmountDueCents     int    `json:"amount_due_cents"`
		TrialDaysRemaining int    `json:"trial_days_remaining"`
	}
	decodeData(t, body, &sub)
	if !sub.BillingEnabled || sub.Status != "trialing" {
		t.Fatalf("expected trialing billing, got %+v", sub)
	}
	if sub.Seats != 1 || sub.PricePerSeatCents != 1500 || sub.AmountDueCents != 1500 {
		t.Fatalf("expected 1 seat * 1500 = 1500, got %+v", sub)
	}
	if sub.TrialDaysRemaining <= 0 {
		t.Fatalf("expected positive trial days remaining, got %d", sub.TrialDaysRemaining)
	}

	// Cancel.
	resp, body = admin.do("PATCH", "/v1/admin/orgs/"+orgID+"/subscription", map[string]any{
		"status": "canceled",
	})
	mustStatus(t, resp, body, http.StatusOK)
	decodeData(t, body, &sub)
	if sub.Status != "canceled" {
		t.Fatalf("expected canceled, got %s", sub.Status)
	}
}
