-- 0021_issue_search: full-text search over issues (Phase 6). A GIN index on a
-- to_tsvector expression keeps title + description searchable without a stored
-- column. to_tsvector with a constant config is IMMUTABLE, so it is indexable.

CREATE INDEX issues_search_idx ON issues
    USING GIN (to_tsvector('english', coalesce(title, '') || ' ' || coalesce(description, '')));
