-- Migration: Add projects table and session-project association
-- Created: 2026-02-23

-- Create projects table
CREATE TABLE IF NOT EXISTS projects (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    work_dir VARCHAR(1024) NOT NULL,
    template VARCHAR(100) COMMENT 'react, go, python, nodejs, custom',
    tech_stack JSON COMMENT 'Array of tech tags',
    metadata JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_user_id (user_id),
    INDEX idx_deleted_at (deleted_at),
    UNIQUE KEY unique_user_name (user_id, name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Add project_id column to sessions table (use stored procedure for idempotency)
DELIMITER //
DROP PROCEDURE IF EXISTS add_project_column//
CREATE PROCEDURE add_project_column()
BEGIN
    -- Check if column exists, if not add it
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = DATABASE()
        AND table_name = 'sessions'
        AND column_name = 'project_id'
    ) THEN
        ALTER TABLE sessions ADD COLUMN project_id VARCHAR(36) NULL AFTER user_id;
    END IF;
END //
DELIMITER ;

CALL add_project_column();
DROP PROCEDURE add_project_column;

-- Add foreign key constraint (use stored procedure for idempotency)
DELIMITER //
DROP PROCEDURE IF EXISTS add_project_fk//
CREATE PROCEDURE add_project_fk()
BEGIN
    -- Check if constraint exists, if not add it
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_schema = DATABASE()
        AND table_name = 'sessions'
        AND constraint_name = 'fk_sessions_project'
    ) THEN
        ALTER TABLE sessions
        ADD CONSTRAINT fk_sessions_project
        FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL;
    END IF;
END //
DELIMITER ;

CALL add_project_fk();
DROP PROCEDURE add_project_fk;

-- Add index (use stored procedure for idempotency)
DELIMITER //
DROP PROCEDURE IF EXISTS add_project_index//
CREATE PROCEDURE add_project_index()
BEGIN
    -- Check if index exists, if not add it
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.statistics
        WHERE table_schema = DATABASE()
        AND table_name = 'sessions'
        AND index_name = 'idx_sessions_project_id'
    ) THEN
        ALTER TABLE sessions ADD INDEX idx_sessions_project_id (project_id);
    END IF;
END //
DELIMITER ;

CALL add_project_index();
DROP PROCEDURE add_project_index;
