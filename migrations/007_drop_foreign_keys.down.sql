-- Restore foreign key constraints (if needed for multi-user mode)

-- Add foreign key back to sessions table
ALTER TABLE sessions
    ADD CONSTRAINT fk_sessions_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Add foreign key back to memories table
ALTER TABLE memories
    ADD CONSTRAINT fk_memories_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Add foreign key back to api_keys table
ALTER TABLE api_keys
    ADD CONSTRAINT fk_api_keys_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
