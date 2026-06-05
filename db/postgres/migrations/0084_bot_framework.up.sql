-- 0084_bot_framework
-- Add bots.framework to select the agent framework backing a bot.

ALTER TABLE bots
  ADD COLUMN IF NOT EXISTS framework TEXT NOT NULL DEFAULT 'memoh';

ALTER TABLE bots
  DROP CONSTRAINT IF EXISTS bots_framework_check;

ALTER TABLE bots
  ADD CONSTRAINT bots_framework_check CHECK (framework IN ('memoh', 'codex'));
