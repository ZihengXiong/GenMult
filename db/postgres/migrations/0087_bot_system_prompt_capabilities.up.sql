-- 0087_bot_system_prompt_capabilities
-- Add system_prompt and capabilities columns to bots table for custom agent configuration.

ALTER TABLE bots
  ADD COLUMN IF NOT EXISTS system_prompt TEXT NOT NULL DEFAULT '';

ALTER TABLE bots
  ADD COLUMN IF NOT EXISTS capabilities TEXT[] NOT NULL DEFAULT '{}';
