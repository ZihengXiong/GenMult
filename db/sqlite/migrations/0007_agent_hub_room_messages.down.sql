-- 0007_agent_hub_room_messages
-- Remove AgentHub room timeline messages.

DROP INDEX IF EXISTS idx_agent_hub_room_messages_room;
DROP TABLE IF EXISTS agent_hub_room_messages;
