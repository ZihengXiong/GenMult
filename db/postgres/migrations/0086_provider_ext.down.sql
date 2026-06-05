-- 0086_provider_ext.down.sql
-- Drop provider_ext column from bots table.
ALTER TABLE bots DROP COLUMN IF EXISTS provider_ext;
