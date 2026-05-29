-- 0006_agent_hub_rooms
-- Remove AgentHub rooms and room agent membership.

DROP INDEX IF EXISTS idx_agent_hub_room_agents_agent;
DROP TABLE IF EXISTS agent_hub_room_agents;
DROP INDEX IF EXISTS idx_agent_hub_rooms_owner;
DROP TABLE IF EXISTS agent_hub_rooms;
