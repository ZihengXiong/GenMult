-- 0008_agent_hub_orchestrator
-- Drop AgentHub orchestrator run/task/attempt/event persistence tables.

DROP TABLE IF EXISTS agent_hub_run_events;
DROP TABLE IF EXISTS agent_hub_task_attempts;
DROP TABLE IF EXISTS agent_hub_task_deps;
DROP TABLE IF EXISTS agent_hub_tasks;
DROP TABLE IF EXISTS agent_hub_runs;
