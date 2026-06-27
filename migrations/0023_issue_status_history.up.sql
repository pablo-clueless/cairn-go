-- 0023_issue_status_history: append-only log of an issue's status over time
-- (Phase 6 reporting). One row is written when an issue is created and on every
-- status change, so burndown and cumulative-flow can be reconstructed by day.
-- (Velocity is derived from current sprint/issue data and needs no history.)

CREATE TABLE issue_status_history (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    issue_id        UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    space_id        UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    status_id       UUID NOT NULL REFERENCES workflow_statuses(id),
    category        TEXT NOT NULL,   -- todo | in_progress | done, as of the change
    changed_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX issue_status_history_space_idx ON issue_status_history (space_id, changed_at);
CREATE INDEX issue_status_history_issue_idx ON issue_status_history (issue_id, changed_at);
