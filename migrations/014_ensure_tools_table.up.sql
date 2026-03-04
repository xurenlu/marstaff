-- Ensure tools table exists (may be missing if init schema was run partially)
CREATE TABLE IF NOT EXISTS tools (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    parameters JSON,
    handler_name VARCHAR(100) NOT NULL,
    skill_id VARCHAR(36),
    enabled BOOLEAN DEFAULT TRUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_skill_id (skill_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
