-- 0011_bot_system_prompt_capabilities
-- Add system_prompt and capabilities columns to bots table for custom agent configuration.

ALTER TABLE bots ADD COLUMN system_prompt TEXT NOT NULL DEFAULT '';
ALTER TABLE bots ADD COLUMN capabilities TEXT NOT NULL DEFAULT '[]';
