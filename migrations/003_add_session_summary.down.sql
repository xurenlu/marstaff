-- Rollback: Remove conversation summary from sessions

-- Remove summary column from sessions table
ALTER TABLE sessions DROP COLUMN summary;
