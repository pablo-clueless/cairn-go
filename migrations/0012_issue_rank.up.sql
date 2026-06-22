-- 0012_issue_rank: fractional ordering for issues (stable drag-and-drop on the
-- board and backlog). Lower rank sorts first; new issues get max(rank)+1024.

ALTER TABLE issues ADD COLUMN rank DOUBLE PRECISION NOT NULL DEFAULT 0;

-- Seed existing issues with a spaced-out rank per space, ordered by creation.
WITH ordered AS (
    SELECT id, row_number() OVER (PARTITION BY space_id ORDER BY created_at) AS rn
    FROM issues
)
UPDATE issues i
SET rank = o.rn * 1024
FROM ordered o
WHERE o.id = i.id;

CREATE INDEX issues_rank_idx ON issues (space_id, rank);
