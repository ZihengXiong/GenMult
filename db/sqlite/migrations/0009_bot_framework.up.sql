-- 0009_bot_framework
-- Add bots.framework to select the agent framework backing a bot.

ALTER TABLE bots ADD COLUMN framework TEXT NOT NULL DEFAULT 'memoh';
