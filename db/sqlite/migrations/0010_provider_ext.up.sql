-- 0010_provider_ext.up.sql
-- Add provider_ext column to bots table for framework extensions.
ALTER TABLE bots ADD COLUMN provider_ext TEXT NOT NULL DEFAULT '{}';
