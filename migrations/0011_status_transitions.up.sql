-- 0011_status_transitions: per-space allowed issue status transitions.
--
-- Workflow semantics:
--   * A space with ZERO rows here has an "open" workflow — any status may
--     transition to any other (this keeps all existing spaces working as-is).
--   * Once a space has at least one row, only the listed transitions (plus the
--     same-status no-op) are permitted; everything else is rejected.
--   * from_status_id NULL means "from any status" (a global transition).

CREATE TABLE status_transitions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    space_id        UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    from_status_id  UUID REFERENCES workflow_statuses(id) ON DELETE CASCADE, -- NULL = any status
    to_status_id    UUID NOT NULL REFERENCES workflow_statuses(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX status_transitions_space_idx ON status_transitions (space_id);

-- One edge per (from, to) within a space. COALESCE folds the NULL "any" source
-- onto a sentinel uuid so global edges still dedupe against each other.
CREATE UNIQUE INDEX status_transitions_unique_idx ON status_transitions (
    space_id,
    COALESCE(from_status_id, '00000000-0000-0000-0000-000000000000'::uuid),
    to_status_id
);

-- No seeding: every existing space starts with an open workflow.
