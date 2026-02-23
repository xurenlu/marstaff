-- AFK Tasks table - main task storage
CREATE TABLE IF NOT EXISTS afk_tasks (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    session_id VARCHAR(36) COMMENT 'Reference session that created this task',
    name VARCHAR(255) NOT NULL,
    description TEXT,

    -- Task type and configuration
    task_type VARCHAR(20) NOT NULL DEFAULT 'scheduled' COMMENT 'scheduled, ai_driven, event_based',
    trigger_config JSON NOT NULL COMMENT 'Cron expression, AI prompt, or event conditions',
    action_config JSON NOT NULL COMMENT 'What to execute when triggered',

    -- Notification configuration
    notification_config JSON COMMENT 'Channels and conditions for notifications',

    -- Task state
    status VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT 'active, paused, disabled, completed, error',
    last_execution_time DATETIME,
    next_execution_time DATETIME,
    execution_count INT DEFAULT 0,
    error_message TEXT,

    -- Metadata
    metadata JSON COMMENT 'Additional task-specific data',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME,

    INDEX idx_user_id (user_id),
    INDEX idx_status (status),
    INDEX idx_next_execution (next_execution_time),
    INDEX idx_task_type (task_type),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Task Execution History table
CREATE TABLE IF NOT EXISTS afk_task_executions (
    id VARCHAR(36) PRIMARY KEY,
    task_id VARCHAR(36) NOT NULL,
    execution_time DATETIME NOT NULL,
    status VARCHAR(20) NOT NULL COMMENT 'success, failed, partial',
    result JSON COMMENT 'Execution result data',
    error_message TEXT,
    triggered_by VARCHAR(100) COMMENT 'What triggered this execution',
    notification_sent BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_task_id (task_id),
    INDEX idx_execution_time (execution_time DESC),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- User Notification Settings table
CREATE TABLE IF NOT EXISTS user_notification_settings (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,

    -- Feishu settings
    feishu_webhook_url VARCHAR(512),
    feishu_enabled BOOLEAN DEFAULT FALSE,

    -- WeChat Work (企业微信) settings
    wecom_webhook_url VARCHAR(512),
    wecom_enabled BOOLEAN DEFAULT FALSE,

    -- Telegram settings
    telegram_chat_id VARCHAR(100),
    telegram_bot_token VARCHAR(512),
    telegram_enabled BOOLEAN DEFAULT FALSE,

    -- Email settings
    email_address VARCHAR(255),
    email_enabled BOOLEAN DEFAULT FALSE,

    -- Web Push settings
    web_push_subscription JSON,
    web_push_enabled BOOLEAN DEFAULT FALSE,

    -- Global preferences
    quiet_hours_start TIME COMMENT 'Start of quiet hours (24h format)',
    quiet_hours_end TIME COMMENT 'End of quiet hours (24h format)',
    quiet_hours_enabled BOOLEAN DEFAULT FALSE,

    metadata JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY unique_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
