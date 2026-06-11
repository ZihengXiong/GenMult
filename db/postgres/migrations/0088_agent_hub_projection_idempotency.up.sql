-- 0088_agent_hub_projection_idempotency
-- Make orchestrator run-event projection idempotent at the insert layer.
-- Every projected room message carries {run_id, event_seq} in its metadata
-- (see internal/agenthub/orchestrator_projector.go roomMessageForEvent), so a
-- partial unique index over those keys lets the insert suppress duplicate
-- projections after a process restart, when the in-memory projection
-- high-water mark starts empty.

-- Remove duplicates left by pre-index restarts, keeping the oldest row of each
-- (run_id, event_seq) pair so the original timeline position is preserved.
DELETE FROM agent_hub_room_messages
WHERE metadata ? 'run_id'
  AND metadata ? 'event_seq'
  AND id NOT IN (
    SELECT (array_agg(id ORDER BY created_at ASC, id ASC))[1]
    FROM agent_hub_room_messages
    WHERE metadata ? 'run_id'
      AND metadata ? 'event_seq'
    GROUP BY metadata->>'run_id', metadata->>'event_seq'
  );

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_hub_room_messages_run_event
  ON agent_hub_room_messages ((metadata->>'run_id'), (metadata->>'event_seq'))
  WHERE metadata ? 'run_id' AND metadata ? 'event_seq';
