-- Add pipelines/workflows table for complex multi-step task automation
CREATE TABLE IF NOT EXISTS pipelines (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL COMMENT 'User who created the pipeline',
    session_id VARCHAR(36) COMMENT 'Associated session ID',
    name VARCHAR(255) NOT NULL COMMENT 'Pipeline name',
    description TEXT COMMENT 'Pipeline description',
    status VARCHAR(50) DEFAULT 'pending' COMMENT 'pending, running, completed, failed, cancelled',
    definition JSON NOT NULL COMMENT 'Pipeline definition (steps, dependencies, etc.)',
    result JSON COMMENT 'Final result of the pipeline',
    error_message TEXT COMMENT 'Error message if failed',
    started_at DATETIME COMMENT 'When execution started',
    completed_at DATETIME COMMENT 'When execution completed',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id),
    INDEX idx_session_id (session_id),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Pipeline/workflow definitions for complex multi-step tasks';

-- Pipeline steps table
CREATE TABLE IF NOT EXISTS pipeline_steps (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    pipeline_id BIGINT UNSIGNED NOT NULL,
    step_key VARCHAR(100) NOT NULL COMMENT 'Unique step identifier within pipeline',
    step_type VARCHAR(50) NOT NULL COMMENT 'task, parallel, conditional, delay, wait',
    step_order INT NOT NULL COMMENT 'Execution order',
    name VARCHAR(255) COMMENT 'Step name',
    config JSON COMMENT 'Step configuration (task_type, params, etc.)',
    dependencies JSON COMMENT 'Array of step keys this step depends on',
    status VARCHAR(50) DEFAULT 'pending' COMMENT 'pending, running, completed, failed, skipped',
    result JSON COMMENT 'Step execution result',
    error_message TEXT COMMENT 'Error message if failed',
    started_at DATETIME COMMENT 'When step started',
    completed_at DATETIME COMMENT 'When step completed',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_pipeline_id (pipeline_id),
    INDEX idx_step_key (pipeline_id, step_key),
    INDEX idx_status (status),
    FOREIGN KEY (pipeline_id) REFERENCES pipelines(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Individual steps within a pipeline';
