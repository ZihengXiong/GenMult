-- 0086_provider_ext.up.sql
-- Add provider_ext column to bots table for framework extensions.
ALTER TABLE bots ADD COLUMN IF NOT EXISTS provider_ext JSONB NOT NULL DEFAULT '{}'::jsonb;
