-- 0085_room_orchestrator
-- Add orchestrator_agent_id to agent_hub_rooms so each room can designate
-- a main (orchestrator) agent for message dispatch.

ALTER TABLE agent_hub_rooms ADD COLUMN IF NOT EXISTS orchestrator_agent_id TEXT NOT NULL DEFAULT '';
