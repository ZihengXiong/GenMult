-- 0006_agent_hub_rooms
-- Persist AgentHub rooms and room agent membership.

CREATE TABLE IF NOT EXISTS agent_hub_rooms (
  id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  short_name TEXT NOT NULL DEFAULT '',
  subtitle TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  privacy TEXT NOT NULL DEFAULT '本地群',
  live TEXT NOT NULL DEFAULT '等待输入',
  accent TEXT NOT NULL DEFAULT 'bg-slate-700',
  status_class TEXT NOT NULL DEFAULT 'bg-slate-500',
  attention INTEGER NOT NULL DEFAULT 0,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_hub_rooms_owner
  ON agent_hub_rooms(owner_user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS agent_hub_room_agents (
  room_id TEXT NOT NULL REFERENCES agent_hub_rooms(id) ON DELETE CASCADE,
  agent_id TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (room_id, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_hub_room_agents_agent
  ON agent_hub_room_agents(agent_id);
