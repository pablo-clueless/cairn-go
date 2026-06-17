-- 0005_spaces_issues: Phase 3 — spaces (projects), issues, and the audit log.

CREATE TABLE spaces (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    key             TEXT NOT NULL,                 -- e.g. "ENG"
    name            TEXT NOT NULL,
    description     TEXT,
    lead_id         UUID REFERENCES users(id) ON DELETE SET NULL,
    issue_seq       INTEGER NOT NULL DEFAULT 0,    -- per-space issue number counter
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, key)
);
CREATE INDEX spaces_org_idx ON spaces (organization_id);

CREATE TABLE issues (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    space_id        UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    number          INTEGER NOT NULL,              -- per-space sequence; key = space.key-number
    type            TEXT NOT NULL DEFAULT 'task'
        CHECK (type IN ('epic', 'story', 'task', 'bug', 'subtask')),
    title           TEXT NOT NULL,
    description     TEXT,
    status          TEXT NOT NULL DEFAULT 'todo'
        CHECK (status IN ('todo', 'in_progress', 'done')),
    priority        TEXT NOT NULL DEFAULT 'medium'
        CHECK (priority IN ('lowest', 'low', 'medium', 'high', 'highest')),
    assignee_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    reporter_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (space_id, number)
);
CREATE INDEX issues_space_idx ON issues (space_id);
CREATE INDEX issues_org_idx ON issues (organization_id);
CREATE INDEX issues_assignee_idx ON issues (assignee_id);

-- Audit log (begins in Phase 3; org-scoped).
CREATE TABLE audit_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    actor_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    action          TEXT NOT NULL,                 -- e.g. "issue.created"
    entity_type     TEXT NOT NULL,                 -- "space" | "issue"
    entity_id       UUID NOT NULL,
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_org_idx ON audit_events (organization_id, created_at DESC);
