-- Migration: Add conversation summary to sessions
-- Created: 2026-02-23

-- Add summary column to sessions table for storing compressed conversation history
-- Safe to run multiple times - will fail if column already exists
ALTER TABLE sessions
ADD COLUMN summary TEXT NULL AFTER system_prompt;
