-- Fix charset for AFK tables to match other tables (utf8mb4_0900_ai_ci)
-- This is required for foreign key constraints to work properly

-- Fix afk_tasks table
ALTER TABLE afk_tasks CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;

-- Fix afk_task_executions table
ALTER TABLE afk_task_executions CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;

-- Fix user_notification_settings table
ALTER TABLE user_notification_settings CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;

-- Add foreign key constraints for AFK tables
ALTER TABLE afk_tasks ADD CONSTRAINT fk_afk_tasks_user FOREIGN KEY (user_id) REFERENCES users(id);
ALTER TABLE afk_tasks ADD CONSTRAINT fk_afk_tasks_session FOREIGN KEY (session_id) REFERENCES sessions(id);
ALTER TABLE afk_task_executions ADD CONSTRAINT fk_afk_task_executions_task FOREIGN KEY (task_id) REFERENCES afk_tasks(id);
