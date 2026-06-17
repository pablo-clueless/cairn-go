-- 0007_workflow_statuses: per-space, user-defined workflow statuses.
-- Replaces the fixed issues.status text with a FK to a configurable status.

CREATE TABLE workflow_statuses (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    space_id        UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    -- category drives board grouping/coloring and "done" semantics (sprint completion).
    category        TEXT NOT NULL DEFAULT 'todo'
        CHECK (category IN ('todo', 'in_progress', 'done')),
    position        INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX workflow_statuses_space_idx ON workflow_statuses (space_id);
CREATE UNIQUE INDEX workflow_statuses_space_name_idx ON workflow_statuses (space_id, lower(name));

-- Seed the three default statuses for every existing space.
INSERT INTO workflow_statuses (organization_id, space_id, name, category, position)
SELECT s.organization_id, s.id, v.name, v.category, v.position
FROM spaces s
CROSS JOIN (VALUES ('To Do', 'todo', 0), ('In Progress', 'in_progress', 1), ('Done', 'done', 2))
    AS v(name, category, position);

-- Point issues at a status, migrating the old text status by category.
ALTER TABLE issues ADD COLUMN status_id UUID REFERENCES workflow_statuses(id) ON DELETE RESTRICT;

UPDATE issues i SET status_id = ws.id
FROM workflow_statuses ws
WHERE ws.space_id = i.space_id AND ws.category = i.status;

ALTER TABLE issues ALTER COLUMN status_id SET NOT NULL;
ALTER TABLE issues DROP COLUMN status;
CREATE INDEX issues_status_idx ON issues (status_id);
