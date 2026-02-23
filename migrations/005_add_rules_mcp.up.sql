-- Add Rules table for system prompt rules
CREATE TABLE IF NOT EXISTS rules (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    content TEXT NOT NULL,
    enabled BOOLEAN DEFAULT TRUE NOT NULL,
    is_active BOOLEAN DEFAULT FALSE NOT NULL,
    is_builtin BOOLEAN DEFAULT FALSE NOT NULL,
    user_id VARCHAR(36),
    category VARCHAR(50),
    tags VARCHAR(255),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    INDEX idx_enabled (enabled),
    INDEX idx_user_id (user_id),
    INDEX idx_deleted_at (deleted_at)
);

-- Add MCP servers table
CREATE TABLE IF NOT EXISTS mcp_servers (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    endpoint VARCHAR(500) NOT NULL,
    enabled BOOLEAN DEFAULT TRUE NOT NULL,
    user_id VARCHAR(36),
    config JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    INDEX idx_enabled (enabled),
    INDEX idx_user_id (user_id),
    INDEX idx_deleted_at (deleted_at)
);

-- Add MCP tools table
CREATE TABLE IF NOT EXISTS mcp_tools (
    id VARCHAR(36) PRIMARY KEY,
    server_id VARCHAR(36) NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    input_schema JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_server_id (server_id),
    FOREIGN KEY (server_id) REFERENCES mcp_servers(id) ON DELETE CASCADE
);

-- Insert default rules
INSERT INTO rules (id, name, description, content, is_active, is_builtin, user_id, category) VALUES
('default-coding', 'Default Coding Assistant', 'Default system prompt for coding assistance',
'You are a helpful coding assistant. You help users write, debug, and understand code. When providing code examples, always explain what the code does and how it works.',
TRUE, TRUE, '', 'coding'),

('default-general', 'Default General Assistant', 'Default system prompt for general assistance',
'You are a helpful AI assistant. You answer questions clearly and concisely. If you''re not sure about something, you say so.',
FALSE, TRUE, '', 'general');

-- Insert sample MCP servers (disabled by default)
INSERT INTO mcp_servers (id, name, description, endpoint, enabled, user_id, config) VALUES
('github-mcp', 'GitHub MCP Server', 'GitHub integration via MCP', 'https://api.github.com/mcp', FALSE, '', '{"auth_type": "token", "headers": {}}'),
('filesystem-mcp', 'Filesystem MCP Server', 'Local filesystem access via MCP', 'http://localhost:3000/mcp', FALSE, '', '{"auth_type": "none"}');
