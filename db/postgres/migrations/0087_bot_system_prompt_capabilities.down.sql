-- 0087_bot_system_prompt_capabilities (rollback)

ALTER TABLE bots DROP COLUMN IF EXISTS capabilities;
ALTER TABLE bots DROP COLUMN IF EXISTS system_prompt;
