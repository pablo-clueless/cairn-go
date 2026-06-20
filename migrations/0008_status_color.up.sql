-- 0008_status_color: add a display color to workflow statuses (board column accents).

ALTER TABLE workflow_statuses ADD COLUMN color TEXT NOT NULL DEFAULT '';

-- Seed sensible defaults for existing statuses by category.
UPDATE workflow_statuses SET color = CASE category
    WHEN 'todo'        THEN '#6B7280'
    WHEN 'in_progress' THEN '#3B82F6'
    WHEN 'done'        THEN '#22C55E'
    ELSE ''
END
WHERE color = '';
