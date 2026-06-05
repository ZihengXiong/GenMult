-- 0085_bot_framework_claudecode
-- Remove the claudecode framework from bots.framework.

ALTER TABLE bots
  DROP CONSTRAINT IF EXISTS bots_framework_check;

ALTER TABLE bots
  ADD CONSTRAINT bots_framework_check CHECK (framework IN ('memoh', 'codex'));
