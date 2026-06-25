-- 0020_attachments: file attachments on issues (Phase 5). File bytes live on
-- disk (ATTACHMENTS_DIR); this table holds metadata + the on-disk storage key.

CREATE TABLE attachments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    issue_id        UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    uploaded_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    filename        TEXT NOT NULL,          -- original client filename
    content_type    TEXT NOT NULL DEFAULT 'application/octet-stream',
    size_bytes      BIGINT NOT NULL,
    storage_key     TEXT NOT NULL,          -- relative path under ATTACHMENTS_DIR
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX attachments_issue_idx ON attachments (issue_id, created_at);
