-- 0024_dashboards: per-user dashboards (Phase 6). A dashboard is a named list of
-- widgets (the widgets blob is opaque to the backend — the frontend renders it).
-- Org-scoped and private to the owning user, mirroring saved_filters.

CREATE TABLE dashboards (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    widgets         JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX dashboards_owner_idx ON dashboards (organization_id, user_id, created_at);
