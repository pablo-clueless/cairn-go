-- 0013_status_wip_limit: per-column work-in-progress limit on workflow statuses.
-- 0 means no limit; a positive value flags the board column when exceeded.

ALTER TABLE workflow_statuses ADD COLUMN wip_limit INT NOT NULL DEFAULT 0;
