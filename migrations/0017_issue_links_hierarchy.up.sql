-- 0017_issue_links_hierarchy: issue relationships (Phase 5).
--
-- Two distinct concepts:
--   1. issue_links — lateral relationships between two issues (blocks / relates /
--      duplicates). Stored as a single directed row; the inverse side is derived
--      at read time (e.g. A "blocks" B means B "is blocked by" A).
--   2. issues.parent_id — the epic↔story↔subtask hierarchy (a self-FK). NULL =
--      top-level. ON DELETE SET NULL so deleting a parent orphans its children
--      rather than cascading the delete.

ALTER TABLE issues
    ADD COLUMN parent_id UUID REFERENCES issues(id) ON DELETE SET NULL;

CREATE INDEX issues_parent_idx ON issues (parent_id);

CREATE TABLE issue_links (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    source_issue_id UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    target_issue_id UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    type            TEXT NOT NULL CHECK (type IN ('blocks', 'relates_to', 'duplicates')),
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT issue_links_no_self CHECK (source_issue_id <> target_issue_id),
    CONSTRAINT issue_links_unique UNIQUE (source_issue_id, target_issue_id, type)
);

CREATE INDEX issue_links_source_idx ON issue_links (source_issue_id);
CREATE INDEX issue_links_target_idx ON issue_links (target_issue_id);
