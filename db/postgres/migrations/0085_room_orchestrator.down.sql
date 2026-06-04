-- 0085_room_orchestrator (rollback)
-- Remove orchestrator_agent_id from agent_hub_rooms.

ALTER TABLE agent_hub_rooms DROP COLUMN IF EXISTS orchestrator_agent_id;
