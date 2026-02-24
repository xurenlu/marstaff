-- Fix character set for existing tables to support UTF-8 (Chinese characters)
-- This migration ensures all text columns use utf8mb4

-- Convert sessions table
ALTER TABLE `sessions` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Convert messages table
ALTER TABLE `messages` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Convert projects table
ALTER TABLE `projects` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Convert afk_tasks table
ALTER TABLE `afk_tasks` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Convert rules table
ALTER TABLE `rules` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Convert mcp_servers table
ALTER TABLE `mcp_servers` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
