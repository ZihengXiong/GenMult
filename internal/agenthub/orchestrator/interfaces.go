package orchestrator

import "context"

// Planner turns a user objective and available room agents into a task DAG.
type Planner interface {
	Plan(ctx context.Context, input PlanInput) (Plan, error)
}

type PlanInput struct {
	RoomID    string
	Objective string
	Agents    []AgentDescriptor
	Metadata  map[string]any
}

// AgentProvider executes one task. Concrete implementations can wrap a Memoh bot,
// Codex bridge, Claude Code bridge, or any future AgentHub adapter.
type AgentProvider interface {
	Name() string
	Capabilities() []string
	Execute(ctx context.Context, req ExecuteTaskRequest) (ExecuteTaskResult, error)
}

// Store is the persistence boundary for the orchestrator core. The API/database
// layer can implement this with sqlc; tests and local demos can use MemoryStore.
type Store interface {
	CreateRun(ctx context.Context, run Run) (Run, error)
	GetRun(ctx context.Context, runID string) (Run, error)
	UpdateRunStatus(ctx context.Context, runID string, status RunStatus) (Run, error)
	ListRunsByStatus(ctx context.Context, statuses ...RunStatus) ([]Run, error)
	// GetLatestRunByRoom returns the most recently updated run for a room, or
	// ErrNotFound if the room has no runs yet.
	GetLatestRunByRoom(ctx context.Context, roomID string) (Run, error)

	CreateTasks(ctx context.Context, runID string, drafts []TaskDraft) ([]Task, []TaskDependency, error)
	ListTasks(ctx context.Context, runID string) ([]Task, error)
	UpdateTaskStatus(ctx context.Context, taskID string, status TaskStatus) (Task, error)
	IncrementTaskAttempt(ctx context.Context, taskID string) (Task, error)
	ListDependencies(ctx context.Context, runID string) ([]TaskDependency, error)

	CreateAttempt(ctx context.Context, attempt TaskAttempt) (TaskAttempt, error)
	CompleteAttempt(ctx context.Context, attemptID string, status AttemptStatus, output map[string]any, errorMessage string, retryable bool) (TaskAttempt, error)
	ListAttempts(ctx context.Context, runID string) ([]TaskAttempt, error)

	AppendEvent(ctx context.Context, event RunEvent) (RunEvent, error)
	ListEvents(ctx context.Context, runID string, afterSeq int64, limit int32) ([]RunEvent, error)
}
