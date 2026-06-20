-- 0009_issue_due_date: Phase 3/4 — optional due date on issues.

ALTER TABLE issues ADD COLUMN due_date DATE;
CREATE INDEX issues_due_date_idx ON issues (due_date) WHERE due_date IS NOT NULL;
