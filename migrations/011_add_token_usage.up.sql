-- Add token usage tracking table
CREATE TABLE IF NOT EXISTS token_usage (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    session_id BIGINT UNSIGNED NULL,
    provider VARCHAR(50) NOT NULL COMMENT 'AI provider (zai, qwen, gemini, deepseek, openai, etc.)',
    model VARCHAR(100) NOT NULL COMMENT 'Model name (e.g., glm-4-flash, qwen-plus, gemini-pro)',
    call_type VARCHAR(50) DEFAULT 'chat' COMMENT 'Call type: chat, stream, vision, thinking',
    prompt_tokens INT UNSIGNED DEFAULT 0 COMMENT 'Input tokens',
    completion_tokens INT UNSIGNED DEFAULT 0 COMMENT 'Output tokens',
    total_tokens INT UNSIGNED DEFAULT 0 COMMENT 'Total tokens',
    estimated_cost DECIMAL(10, 6) DEFAULT 0 COMMENT 'Estimated cost in USD',
    metadata JSON COMMENT 'Additional metadata (latency, error info, etc.)',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT 'Record timestamp',
    INDEX idx_session_id (session_id),
    INDEX idx_provider_model (provider, model),
    INDEX idx_created_at (created_at),
    INDEX idx_created_date (DATE(created_at))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Token usage tracking for AI model calls';
