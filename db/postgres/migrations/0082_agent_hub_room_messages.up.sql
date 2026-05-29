-- 0082_agent_hub_room_messages
-- Persist AgentHub room timeline messages.

CREATE TABLE IF NOT EXISTS agent_hub_room_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  room_id UUID NOT NULL REFERENCES agent_hub_rooms(id) ON DELETE CASCADE,
  sender_id TEXT NOT NULL DEFAULT '',
  sender_type TEXT NOT NULL DEFAULT 'user',
  sender_name TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'message',
  title TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_agent_hub_room_messages_room
  ON agent_hub_room_messages(room_id, created_at ASC);
