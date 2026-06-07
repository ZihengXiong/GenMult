-- name: CreateAgentHubRoom :one
INSERT INTO agent_hub_rooms (
  owner_user_id,
  name,
  short_name,
  subtitle,
  summary,
  privacy,
  live,
  accent,
  status_class,
  attention,
  metadata,
  orchestrator_agent_id
)
VALUES (
  sqlc.arg(owner_user_id),
  sqlc.arg(name),
  sqlc.arg(short_name),
  sqlc.arg(subtitle),
  sqlc.arg(summary),
  sqlc.arg(privacy),
  sqlc.arg(live),
  sqlc.arg(accent),
  sqlc.arg(status_class),
  sqlc.arg(attention),
  sqlc.arg(metadata),
  sqlc.arg(orchestrator_agent_id)
)
RETURNING id, owner_user_id, name, short_name, subtitle, summary, privacy, live, accent, status_class, attention, metadata, orchestrator_agent_id, created_at, updated_at;

-- name: GetAgentHubRoom :one
SELECT id, owner_user_id, name, short_name, subtitle, summary, privacy, live, accent, status_class, attention, metadata, orchestrator_agent_id, created_at, updated_at
FROM agent_hub_rooms
WHERE id = sqlc.arg(id)
  AND owner_user_id = sqlc.arg(owner_user_id);

-- name: ListAgentHubRooms :many
SELECT id, owner_user_id, name, short_name, subtitle, summary, privacy, live, accent, status_class, attention, metadata, orchestrator_agent_id, created_at, updated_at
FROM agent_hub_rooms
WHERE owner_user_id = sqlc.arg(owner_user_id)
ORDER BY updated_at DESC, created_at DESC;

-- name: UpdateAgentHubRoom :one
UPDATE agent_hub_rooms
SET name = sqlc.arg(name),
    short_name = sqlc.arg(short_name),
    subtitle = sqlc.arg(subtitle),
    summary = sqlc.arg(summary),
    privacy = sqlc.arg(privacy),
    live = sqlc.arg(live),
    accent = sqlc.arg(accent),
    status_class = sqlc.arg(status_class),
    attention = sqlc.arg(attention),
    metadata = sqlc.arg(metadata),
    orchestrator_agent_id = sqlc.arg(orchestrator_agent_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND owner_user_id = sqlc.arg(owner_user_id)
RETURNING id, owner_user_id, name, short_name, subtitle, summary, privacy, live, accent, status_class, attention, metadata, orchestrator_agent_id, created_at, updated_at;

-- name: DeleteAgentHubRoom :exec
DELETE FROM agent_hub_rooms
WHERE id = sqlc.arg(id)
  AND owner_user_id = sqlc.arg(owner_user_id);

-- name: AddAgentHubRoomAgent :exec
INSERT INTO agent_hub_room_agents (room_id, agent_id)
VALUES (sqlc.arg(room_id), sqlc.arg(agent_id))
ON CONFLICT (room_id, agent_id) DO NOTHING;

-- name: DeleteAgentHubRoomAgent :exec
DELETE FROM agent_hub_room_agents
WHERE room_id = sqlc.arg(room_id)
  AND agent_id = sqlc.arg(agent_id);

-- name: ListAgentHubRoomAgentsByOwner :many
SELECT a.room_id, a.agent_id, a.created_at
FROM agent_hub_room_agents a
JOIN agent_hub_rooms r ON r.id = a.room_id
WHERE r.owner_user_id = sqlc.arg(owner_user_id)
ORDER BY a.created_at ASC;

-- name: CreateAgentHubRoomMessage :one
INSERT INTO agent_hub_room_messages (
  room_id,
  sender_id,
  sender_type,
  sender_name,
  kind,
  title,
  body,
  metadata
)
VALUES (
  sqlc.arg(room_id),
  sqlc.arg(sender_id),
  sqlc.arg(sender_type),
  sqlc.arg(sender_name),
  sqlc.arg(kind),
  sqlc.arg(title),
  sqlc.arg(body),
  sqlc.arg(metadata)
)
RETURNING id, room_id, sender_id, sender_type, sender_name, kind, title, body, metadata, created_at;

-- name: ListAgentHubRoomMessages :many
SELECT m.id, m.room_id, m.sender_id, m.sender_type, m.sender_name, m.kind, m.title, m.body, m.metadata, m.created_at
FROM agent_hub_room_messages m
JOIN agent_hub_rooms r ON r.id = m.room_id
WHERE m.room_id = sqlc.arg(room_id)
  AND r.owner_user_id = sqlc.arg(owner_user_id)
ORDER BY m.created_at ASC, m.id ASC
LIMIT sqlc.arg(limit_count);
