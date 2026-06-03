-- 0084_bot_framework
-- Add bots.framework to select the agent framework backing a bot
-- (memoh = built-in agent, claudecode/codex = CLI-backed runtimes).

ALTER TABLE bots
  ADD COLUMN IF NOT EXISTS framework TEXT NOT NULL DEFAULT 'memoh';

ALTER TABLE bots
  ADD CONSTRAINT bots_framework_check CHECK (framework IN ('memoh', 'claudecode', 'codex'));
