-- 0014_documents: Confluence-style documents within a space (pages & live docs),
-- arranged as a tree via self-referential parent_id.
-- (Renumbered from 0011 to resolve a duplicate version with 0011_status_transitions.)

CREATE TABLE documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    space_id        UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    parent_id       UUID REFERENCES documents(id) ON DELETE CASCADE,
    title           TEXT NOT NULL DEFAULT '',
    type            TEXT NOT NULL DEFAULT 'page'
        CHECK (type IN ('page', 'live', 'whiteboard')),
    status          TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published')),
    content         TEXT NOT NULL DEFAULT '',
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX documents_space_idx ON documents (space_id);
CREATE INDEX documents_parent_idx ON documents (parent_id);
