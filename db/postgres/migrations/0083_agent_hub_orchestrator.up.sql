-- 0083_agent_hub_orchestrator
-- Add AgentHub orchestrator run/task/attempt/event persistence tables.

CREATE TABLE IF NOT EXISTS agent_hub_runs (
  id UUID PRIMARY KEY,
  room_id UUID NOT NULL REFERENCES agent_hub_rooms(id) ON DELETE CASCADE,
  trigger_message_id TEXT NOT NULL DEFAULT '',
  objective TEXT NOT NULL,
  status TEXT NOT NULL,
  planner_version TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at_ms BIGINT NOT NULL,
  updated_at_ms BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_hub_runs_room_updated
  ON agent_hub_runs(room_id, updated_at_ms DESC);

CREATE TABLE IF NOT EXISTS agent_hub_tasks (
  id UUID PRIMARY KEY,
  run_id UUID NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
  parent_task_id UUID,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  assigned_agent_id TEXT NOT NULL DEFAULT '',
  provider_name TEXT NOT NULL DEFAULT '',
  priority INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  timeout_ms BIGINT NOT NULL DEFAULT 0,
  max_retries INTEGER NOT NULL DEFAULT 0,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  idempotency_key TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at_ms BIGINT NOT NULL,
  updated_at_ms BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_hub_tasks_run_priority
  ON agent_hub_tasks(run_id, status, priority DESC, created_at_ms ASC);

CREATE TABLE IF NOT EXISTS agent_hub_task_deps (
  run_id UUID NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
  task_id UUID NOT NULL REFERENCES agent_hub_tasks(id) ON DELETE CASCADE,
  depends_on_task_id UUID NOT NULL REFERENCES agent_hub_tasks(id) ON DELETE CASCADE,
  PRIMARY KEY (task_id, depends_on_task_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_hub_task_deps_run
  ON agent_hub_task_deps(run_id, task_id);

CREATE TABLE IF NOT EXISTS agent_hub_task_attempts (
  id UUID PRIMARY KEY,
  run_id UUID NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
  task_id UUID NOT NULL REFERENCES agent_hub_tasks(id) ON DELETE CASCADE,
  attempt_no INTEGER NOT NULL,
  provider_name TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  input_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  output_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message TEXT NOT NULL DEFAULT '',
  retryable BOOLEAN NOT NULL DEFAULT false,
  started_at_ms BIGINT NOT NULL,
  finished_at_ms BIGINT,
  idempotency_key TEXT NOT NULL,
  UNIQUE (task_id, attempt_no)
);

CREATE INDEX IF NOT EXISTS idx_agent_hub_task_attempts_run
  ON agent_hub_task_attempts(run_id, task_id, attempt_no DESC);

CREATE TABLE IF NOT EXISTS agent_hub_run_events (
  id UUID PRIMARY KEY,
  run_id UUID NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
  task_id UUID,
  seq BIGINT NOT NULL,
  type TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at_ms BIGINT NOT NULL,
  UNIQUE (run_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_agent_hub_run_events_seq
  ON agent_hub_run_events(run_id, seq ASC);
