-- 0010_bot_framework_claudecode
-- Existing SQLite incremental migrations do not add a framework CHECK
-- constraint, so no live schema change is required here. The SQLite baseline
-- in 0001_init.up.sql is updated to include claudecode for fresh databases.

SELECT 1;
