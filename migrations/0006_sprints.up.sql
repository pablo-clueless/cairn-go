-- 0006_sprints: Phase 4 — sprints for agile planning.

CREATE TABLE sprints (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    space_id        UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    goal            TEXT,
    status          TEXT NOT NULL DEFAULT 'planned'
        CHECK (status IN ('planned', 'active', 'completed')),
    start_date      TIMESTAMPTZ,
    end_date        TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sprints_space_idx ON sprints (space_id);

-- Issues belong to the backlog when sprint_id IS NULL, or to a sprint otherwise.
ALTER TABLE issues ADD COLUMN sprint_id UUID REFERENCES sprints(id) ON DELETE SET NULL;
CREATE INDEX issues_sprint_idx ON issues (sprint_id);
