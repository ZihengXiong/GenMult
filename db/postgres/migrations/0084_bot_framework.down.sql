-- 0084_bot_framework
-- Remove bots.framework.

ALTER TABLE bots
  DROP CONSTRAINT IF EXISTS bots_framework_check;

ALTER TABLE bots
  DROP COLUMN IF EXISTS framework;
