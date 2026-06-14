package model

import "time"

// Subscription statuses.
const (
	SubInactive = "inactive" // billing not enabled for the org
	SubTrialing = "trialing" // within the free trial
	SubActive   = "active"   // paid/active period
	SubPastDue  = "past_due" // payment failed (future, with Stripe)
	SubCanceled = "canceled" // canceled
)

// Subscription is an organization's per-seat billing record. Seats are derived
// from the org's membership count, not stored.
type Subscription struct {
	ID                 string     `json:"id"`
	OrganizationID     string     `json:"organization_id"`
	BillingEnabled     bool       `json:"billing_enabled"`
	Status             string     `json:"status"`
	Plan               string     `json:"plan"`
	PricePerSeatCents  int        `json:"price_per_seat_cents"`
	Currency           string     `json:"currency"`
	TrialDays          int        `json:"trial_days"`
	TrialEndsAt        *time.Time `json:"trial_ends_at,omitempty"`
	CurrentPeriodStart *time.Time `json:"current_period_start,omitempty"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end,omitempty"`
	CanceledAt         *time.Time `json:"canceled_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// AppSettings holds operator-tunable global settings.
type AppSettings struct {
	DefaultTrialDays int       `json:"default_trial_days"`
	UpdatedAt        time.Time `json:"updated_at"`
}
