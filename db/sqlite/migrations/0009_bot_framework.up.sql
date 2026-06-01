-- 0009_bot_framework
-- Sync SQLite schema with PostgreSQL 0084: add bots.framework to select the
-- agent framework backing a bot (memoh built-in agent, or claudecode/codex
-- CLI-backed runtimes). Validation of allowed values is enforced in the
-- application layer.

ALTER TABLE bots ADD COLUMN framework TEXT NOT NULL DEFAULT 'memoh';
