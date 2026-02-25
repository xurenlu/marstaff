-- Add is_main_session for sandbox: true=user direct chat, false=group/channel (sandbox applies)
ALTER TABLE sessions ADD COLUMN is_main_session BOOLEAN DEFAULT TRUE COMMENT 'true=direct chat; false=group/channel (sandbox applies when non-main)' AFTER pending_tasks;
