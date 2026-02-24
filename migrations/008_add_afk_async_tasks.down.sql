-- Rollback AFK async tasks migration

-- Remove columns from sessions table (order matters: last added first removed)
ALTER TABLE sessions DROP COLUMN IF EXISTS summary_model;
ALTER TABLE sessions DROP COLUMN IF EXISTS pending_tasks;
ALTER TABLE sessions DROP COLUMN IF EXISTS afk_since;
ALTER TABLE sessions DROP COLUMN IF EXISTS is_afk_mode;

-- Remove columns from afk_tasks table
ALTER TABLE afk_tasks DROP COLUMN IF EXISTS result_url;
ALTER TABLE afk_tasks DROP COLUMN IF EXISTS async_task_config;
