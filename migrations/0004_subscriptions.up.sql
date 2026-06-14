-- 0004_subscriptions: per-seat subscriptions with a configurable free trial,
-- a platform-admin role, and a global settings singleton.

-- Platform operators (super-admins) manage billing across all orgs.
ALTER TABLE users ADD COLUMN is_platform_admin BOOLEAN NOT NULL DEFAULT false;

-- Global, operator-tunable settings. Single row enforced via the boolean PK.
CREATE TABLE app_settings (
    id                 BOOLEAN PRIMARY KEY DEFAULT true CHECK (id),
    default_trial_days INTEGER NOT NULL DEFAULT 14 CHECK (default_trial_days >= 0),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO app_settings (id) VALUES (true);

-- One subscription per organization. Seats are derived from membership count
-- (per-seat, auto), so they are NOT stored here.
CREATE TABLE subscriptions (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id      UUID NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
    billing_enabled      BOOLEAN NOT NULL DEFAULT false,
    status               TEXT NOT NULL DEFAULT 'inactive'
        CHECK (status IN ('inactive', 'trialing', 'active', 'past_due', 'canceled')),
    plan                 TEXT NOT NULL DEFAULT 'per_seat',
    price_per_seat_cents INTEGER NOT NULL DEFAULT 0 CHECK (price_per_seat_cents >= 0),
    currency             TEXT NOT NULL DEFAULT 'USD',
    trial_days           INTEGER NOT NULL DEFAULT 14 CHECK (trial_days >= 0),
    trial_ends_at        TIMESTAMPTZ,
    current_period_start TIMESTAMPTZ,
    current_period_end   TIMESTAMPTZ,
    canceled_at          TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Backfill an inactive subscription for every existing organization.
INSERT INTO subscriptions (organization_id) SELECT id FROM organizations;
