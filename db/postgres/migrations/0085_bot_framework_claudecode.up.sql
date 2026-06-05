-- 0085_bot_framework_claudecode
-- Allow the claudecode framework in bots.framework.

ALTER TABLE bots
  DROP CONSTRAINT IF EXISTS bots_framework_check;

ALTER TABLE bots
  ADD CONSTRAINT bots_framework_check CHECK (framework IN ('memoh', 'claudecode', 'codex'));
