-- Rollback: Remove projects table and session-project association

-- Drop project_id foreign key and column from sessions
ALTER TABLE sessions
DROP FOREIGN KEY IF EXISTS fk_sessions_project,
DROP INDEX IF EXISTS idx_sessions_project_id,
DROP COLUMN IF EXISTS project_id;

-- Drop projects table
DROP TABLE IF EXISTS projects;
