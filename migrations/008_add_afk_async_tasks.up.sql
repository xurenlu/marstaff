-- Add async task support to AFK tasks table
ALTER TABLE afk_tasks ADD COLUMN async_task_config JSON COMMENT 'Configuration for async tasks (video/image generation)' AFTER trigger_config;

-- Add result_url column for storing completed async task results
ALTER TABLE afk_tasks ADD COLUMN result_url VARCHAR(512) COMMENT 'URL of the result when task completes' AFTER error_message;

-- Add AFK mode columns to sessions table
ALTER TABLE sessions ADD COLUMN is_afk_mode BOOLEAN DEFAULT FALSE COMMENT 'Whether the session is in AFK (waiting) mode' AFTER model;
ALTER TABLE sessions ADD COLUMN afk_since DATETIME COMMENT 'When the session entered AFK mode' AFTER is_afk_mode;
ALTER TABLE sessions ADD COLUMN pending_tasks INT DEFAULT 0 COMMENT 'Number of pending async tasks' AFTER afk_since;

-- Add summary_model column (if not exists)
ALTER TABLE sessions ADD COLUMN summary_model VARCHAR(100) DEFAULT '' COMMENT 'Model used for session summaries' AFTER pending_tasks;
