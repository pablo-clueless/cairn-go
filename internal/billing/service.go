// Package billing implements per-seat subscriptions with a configurable free
// trial. Seats are derived from organization membership (per-seat, auto). There
// is no payment processing yet — this is the model + lifecycle only.
package billing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"cairn/internal/model"
	"cairn/internal/store"
)

// ErrInvalidStatus is returned when an unknown status is supplied.
var ErrInvalidStatus = errors.New("billing: invalid status")

// ErrInvalidCurrency is returned when an unsupported currency is supplied.
var ErrInvalidCurrency = errors.New("billing: invalid currency")

// SupportedCurrencies are the ISO-4217 codes a tenant may bill in.
var SupportedCurrencies = []string{"USD", "EUR", "GBP", "NGN", "GHS", "KES", "ZAR", "CAD", "AUD"}

// ValidCurrency reports whether code is a supported currency.
func ValidCurrency(code string) bool {
	return slices.Contains(SupportedCurrencies, code)
}

// defaultPeriod is the assumed billing period once a trial converts to active.
const defaultPeriod = 30 * 24 * time.Hour

// Service implements subscription workflows.
type Service struct {
	store             *store.DB
	defaultPriceCents int
	defaultCurrency   string
}

// NewService builds a billing Service.
func NewService(db *store.DB, defaultPriceCents int, defaultCurrency string) *Service {
	if defaultCurrency == "" {
		defaultCurrency = "NGN"
	}
	return &Service{store: db, defaultPriceCents: defaultPriceCents, defaultCurrency: defaultCurrency}
}

// View is a subscription plus derived, per-seat fields.
type View struct {
	*model.Subscription
	Seats              int  `json:"seats"`
	AmountDueCents     int  `json:"amount_due_cents"`
	TrialDaysRemaining int  `json:"trial_days_remaining"`
	TrialExpired       bool `json:"trial_expired"`
}

// InitializeForOrg creates the subscription row for a new org. When billing is
// enabled the free trial starts immediately using the global default length.
func (s *Service) InitializeForOrg(ctx context.Context, orgID string, billingEnabled bool) (*model.Subscription, error) {
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}

	sub := &model.Subscription{
		OrganizationID:    orgID,
		BillingEnabled:    billingEnabled,
		Plan:              "per_seat",
		PricePerSeatCents: s.defaultPriceCents,
		Currency:          s.defaultCurrency,
		TrialDays:         settings.DefaultTrialDays,
		Status:            model.SubInactive,
	}
	if billingEnabled {
		startTrial(sub, settings.DefaultTrialDays)
	}
	return s.store.CreateSubscription(ctx, sub)
}

// Get returns an org's subscription with derived fields, lazily creating an
// inactive subscription if one is somehow missing.
func (s *Service) Get(ctx context.Context, orgID string) (*View, error) {
	sub, err := s.store.GetSubscriptionByOrg(ctx, orgID)
	if errors.Is(err, store.ErrNotFound) {
		sub, err = s.InitializeForOrg(ctx, orgID, false)
	}
	if err != nil {
		return nil, err
	}
	seats, err := s.store.CountMembers(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return s.view(sub, seats), nil
}

// AdminUpdateParams carries optional platform-admin changes.
type AdminUpdateParams struct {
	BillingEnabled    *bool
	Status            *string
	TrialDays         *int
	PricePerSeatCents *int
	Currency          *string
}

// AdminUpdate applies platform-admin changes to an org's subscription.
func (s *Service) AdminUpdate(ctx context.Context, orgID string, p AdminUpdateParams) (*View, error) {
	sub, err := s.store.GetSubscriptionByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}

	if p.PricePerSeatCents != nil {
		sub.PricePerSeatCents = *p.PricePerSeatCents
	}
	if p.TrialDays != nil {
		sub.TrialDays = *p.TrialDays
	}
	if p.Currency != nil {
		if !ValidCurrency(*p.Currency) {
			return nil, ErrInvalidCurrency
		}
		sub.Currency = *p.Currency
	}

	if p.BillingEnabled != nil {
		switch {
		case *p.BillingEnabled && !sub.BillingEnabled:
			sub.BillingEnabled = true
			startTrial(sub, sub.TrialDays)
		case !*p.BillingEnabled:
			sub.BillingEnabled = false
			sub.Status = model.SubInactive
			sub.TrialEndsAt = nil
		}
	}

	if p.Status != nil {
		if !validStatus(*p.Status) {
			return nil, ErrInvalidStatus
		}
		applyStatus(sub, *p.Status)
	}

	// If trial length changed while trialing, re-anchor the trial end to now.
	if p.TrialDays != nil && sub.Status == model.SubTrialing {
		end := time.Now().AddDate(0, 0, sub.TrialDays)
		sub.TrialEndsAt = &end
	}

	updated, err := s.store.UpdateSubscription(ctx, sub)
	if err != nil {
		return nil, err
	}
	seats, err := s.store.CountMembers(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return s.view(updated, seats), nil
}

// AdminOrgItem pairs an organization with its subscription (platform-admin list).
type AdminOrgItem struct {
	Organization model.Organization `json:"organization"`
	Subscription *View              `json:"subscription"`
}

// AdminList returns every organization with its subscription view.
func (s *Service) AdminList(ctx context.Context) ([]AdminOrgItem, error) {
	orgs, err := s.store.ListAllOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]AdminOrgItem, 0, len(orgs))
	for i := range orgs {
		view, err := s.Get(ctx, orgs[i].ID)
		if err != nil {
			return nil, err
		}
		items = append(items, AdminOrgItem{Organization: orgs[i], Subscription: view})
	}
	return items, nil
}

// AdminGet returns a single organization with its subscription view.
func (s *Service) AdminGet(ctx context.Context, orgID string) (*AdminOrgItem, error) {
	org, err := s.store.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return nil, err
	}
	view, err := s.Get(ctx, org.ID)
	if err != nil {
		return nil, err
	}
	return &AdminOrgItem{Organization: *org, Subscription: view}, nil
}

// Settings returns the global settings.
func (s *Service) Settings(ctx context.Context) (model.AppSettings, error) {
	return s.store.GetSettings(ctx)
}

// UpdateTrialDays sets the global default free-trial length.
func (s *Service) UpdateTrialDays(ctx context.Context, days int) (model.AppSettings, error) {
	if days < 0 {
		return model.AppSettings{}, fmt.Errorf("billing: trial days must be >= 0")
	}
	return s.store.UpdateDefaultTrialDays(ctx, days)
}

func (s *Service) view(sub *model.Subscription, seats int) *View {
	v := &View{Subscription: sub, Seats: seats}

	if sub.BillingEnabled && (sub.Status == model.SubTrialing || sub.Status == model.SubActive || sub.Status == model.SubPastDue) {
		v.AmountDueCents = seats * sub.PricePerSeatCents
	}

	if sub.Status == model.SubTrialing && sub.TrialEndsAt != nil {
		now := time.Now()
		if now.Before(*sub.TrialEndsAt) {
			v.TrialDaysRemaining = int(math.Ceil(sub.TrialEndsAt.Sub(now).Hours() / 24))
		} else {
			v.TrialExpired = true
		}
	}
	return v
}

func startTrial(sub *model.Subscription, days int) {
	now := time.Now()
	end := now.AddDate(0, 0, days)
	sub.Status = model.SubTrialing
	sub.TrialDays = days
	sub.TrialEndsAt = &end
	sub.CurrentPeriodStart = nil
	sub.CurrentPeriodEnd = nil
	sub.CanceledAt = nil
}

func applyStatus(sub *model.Subscription, status string) {
	sub.Status = status
	now := time.Now()
	switch status {
	case model.SubActive:
		sub.TrialEndsAt = nil
		sub.CanceledAt = nil
		start := now
		end := now.Add(defaultPeriod)
		sub.CurrentPeriodStart = &start
		sub.CurrentPeriodEnd = &end
	case model.SubCanceled:
		sub.CanceledAt = &now
	}
}

func validStatus(status string) bool {
	switch status {
	case model.SubInactive, model.SubTrialing, model.SubActive, model.SubPastDue, model.SubCanceled:
		return true
	default:
		return false
	}
}
