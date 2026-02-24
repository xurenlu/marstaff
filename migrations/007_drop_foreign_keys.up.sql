-- Drop foreign key constraints for single-user mode simplicity
-- This allows using 'default' as user_id without requiring a matching user record

-- Drop foreign key from sessions table
ALTER TABLE sessions DROP FOREIGN KEY fk_sessions_user;

-- Drop foreign key from memories table
ALTER TABLE memories DROP FOREIGN KEY fk_memories_user;

