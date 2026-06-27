-- 0022_saved_filters: per-user saved issue filters (Phase 6). The criteria blob
-- is opaque to the backend — the frontend writes and applies it. Org-scoped and
-- private to the owning user.

CREATE TABLE saved_filters (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    criteria        JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_starred      BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX saved_filters_owner_idx ON saved_filters (organization_id, user_id, created_at);
