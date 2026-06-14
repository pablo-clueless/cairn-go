package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

const subscriptionColumns = `id::text, organization_id::text, billing_enabled, status, plan,
	price_per_seat_cents, currency, trial_days, trial_ends_at, current_period_start,
	current_period_end, canceled_at, created_at, updated_at`

func scanSubscription(row pgx.Row) (*model.Subscription, error) {
	s := &model.Subscription{}
	err := row.Scan(
		&s.ID, &s.OrganizationID, &s.BillingEnabled, &s.Status, &s.Plan,
		&s.PricePerSeatCents, &s.Currency, &s.TrialDays, &s.TrialEndsAt, &s.CurrentPeriodStart,
		&s.CurrentPeriodEnd, &s.CanceledAt, &s.CreatedAt, &s.UpdatedAt,
	)
	return s, err
}

// CreateSubscription inserts an organization's subscription row.
func (db *DB) CreateSubscription(ctx context.Context, s *model.Subscription) (*model.Subscription, error) {
	created, err := scanSubscription(db.Pool.QueryRow(ctx, `
		INSERT INTO subscriptions
			(organization_id, billing_enabled, status, plan, price_per_seat_cents, currency, trial_days, trial_ends_at, current_period_start, current_period_end)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+subscriptionColumns,
		s.OrganizationID, s.BillingEnabled, s.Status, s.Plan, s.PricePerSeatCents,
		s.Currency, s.TrialDays, s.TrialEndsAt, s.CurrentPeriodStart, s.CurrentPeriodEnd,
	))
	if err != nil {
		return nil, fmt.Errorf("store: create subscription: %w", err)
	}
	return created, nil
}

// GetSubscriptionByOrg returns an org's subscription. ErrNotFound if absent.
func (db *DB) GetSubscriptionByOrg(ctx context.Context, orgID string) (*model.Subscription, error) {
	s, err := scanSubscription(db.Pool.QueryRow(ctx,
		`SELECT `+subscriptionColumns+` FROM subscriptions WHERE organization_id = $1::uuid`, orgID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get subscription: %w", err)
	}
	return s, nil
}

// UpdateSubscription persists mutable subscription fields.
func (db *DB) UpdateSubscription(ctx context.Context, s *model.Subscription) (*model.Subscription, error) {
	updated, err := scanSubscription(db.Pool.QueryRow(ctx, `
		UPDATE subscriptions SET
			billing_enabled = $2, status = $3, price_per_seat_cents = $4, trial_days = $5,
			trial_ends_at = $6, current_period_start = $7, current_period_end = $8,
			canceled_at = $9, currency = $10, updated_at = now()
		WHERE organization_id = $1::uuid
		RETURNING `+subscriptionColumns,
		s.OrganizationID, s.BillingEnabled, s.Status, s.PricePerSeatCents, s.TrialDays,
		s.TrialEndsAt, s.CurrentPeriodStart, s.CurrentPeriodEnd, s.CanceledAt, s.Currency,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: update subscription: %w", err)
	}
	return updated, nil
}

// GetSettings returns the global app settings (single row).
func (db *DB) GetSettings(ctx context.Context) (model.AppSettings, error) {
	var s model.AppSettings
	err := db.Pool.QueryRow(ctx,
		`SELECT default_trial_days, updated_at FROM app_settings WHERE id = true`,
	).Scan(&s.DefaultTrialDays, &s.UpdatedAt)
	if err != nil {
		return s, fmt.Errorf("store: get settings: %w", err)
	}
	return s, nil
}

// UpdateDefaultTrialDays updates the global default trial length.
func (db *DB) UpdateDefaultTrialDays(ctx context.Context, days int) (model.AppSettings, error) {
	var s model.AppSettings
	err := db.Pool.QueryRow(ctx,
		`UPDATE app_settings SET default_trial_days = $1, updated_at = now() WHERE id = true
		 RETURNING default_trial_days, updated_at`, days,
	).Scan(&s.DefaultTrialDays, &s.UpdatedAt)
	if err != nil {
		return s, fmt.Errorf("store: update settings: %w", err)
	}
	return s, nil
}
