-- 0001_init
-- Baseline migration. Confirms the migration runner works end-to-end.
-- Real domain schema (users, organizations, projects, issues, ...) begins in Phase 1.
-- Postgres 13+ provides gen_random_uuid() in core, so no extension is required here.
SELECT 1;
