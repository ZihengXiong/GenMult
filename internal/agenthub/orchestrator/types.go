package orchestrator

import "time"

// RunStatus describes the lifecycle of one AgentHub orchestration run.
type RunStatus string

const (
	RunStatusDraft       RunStatus = "draft"
	RunStatusPlanning    RunStatus = "planning"
	RunStatusDispatching RunStatus = "dispatching"
	RunStatusCollecting  RunStatus = "collecting"
	RunStatusCompleted   RunStatus = "completed"
	RunStatusFailed      RunStatus = "failed"
	RunStatusCancelled   RunStatus = "cancelled"
)

// TaskStatus describes the lifecycle of one task node inside a run DAG.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusReady     TaskStatus = "ready"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusBlocked   TaskStatus = "blocked"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// AttemptStatus describes one execution attempt for a task.
type AttemptStatus string

const (
	AttemptStatusRunning   AttemptStatus = "running"
	AttemptStatusSucceeded AttemptStatus = "succeeded"
	AttemptStatusFailed    AttemptStatus = "failed"
	AttemptStatusTimedOut  AttemptStatus = "timed_out"
	AttemptStatusCancelled AttemptStatus = "cancelled"
)

// EventType is the append-only audit/projection event type emitted by the orchestrator.
type EventType string

const (
	EventRunCreated       EventType = "run_created"
	EventRunPlanned       EventType = "run_planned"
	EventRunStatusChanged EventType = "run_status_changed"
	EventTaskCreated      EventType = "task_created"
	EventTaskReady        EventType = "task_ready"
	EventTaskDispatched   EventType = "task_dispatched"
	EventTaskSucceeded    EventType = "task_succeeded"
	EventTaskFailed       EventType = "task_failed"
	EventTaskBlocked      EventType = "task_blocked"
	EventTaskRetried      EventType = "task_retried"
	EventTaskTimedOut     EventType = "task_timed_out"
	EventTaskCancelled    EventType = "task_cancelled"

	// EventAgentToolCall is emitted when a provider's CLI subprocess invokes a tool.
	EventAgentToolCall EventType = "agent_tool_call"
	// EventAgentOutput is emitted when a provider's CLI subprocess produces text output.
	EventAgentOutput EventType = "agent_output"
)

// Run is one user-triggered orchestration lifecycle in an AgentHub room.
type Run struct {
	ID               string         `json:"id"`
	RoomID           string         `json:"room_id"`
	TriggerMessageID string         `json:"trigger_message_id,omitempty"`
	Objective        string         `json:"objective"`
	Status           RunStatus      `json:"status"`
	PlannerVersion   string         `json:"planner_version"`
	CreatedBy        string         `json:"created_by,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// Task is a node in the run DAG.
type Task struct {
	ID              string         `json:"id"`
	RunID           string         `json:"run_id"`
	ParentTaskID    string         `json:"parent_task_id,omitempty"`
	Title           string         `json:"title"`
	Description     string         `json:"description"`
	AssignedAgentID string         `json:"assigned_agent_id,omitempty"`
	ProviderName    string         `json:"provider_name,omitempty"`
	Priority        int            `json:"priority"`
	Status          TaskStatus     `json:"status"`
	Timeout         time.Duration  `json:"timeout"`
	MaxRetries      int            `json:"max_retries"`
	AttemptCount    int            `json:"attempt_count"`
	IdempotencyKey  string         `json:"idempotency_key,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// TaskDependency means TaskID may run only after DependsOnTaskID succeeds.
type TaskDependency struct {
	RunID           string `json:"run_id"`
	TaskID          string `json:"task_id"`
	DependsOnTaskID string `json:"depends_on_task_id"`
}

// TaskAttempt stores one provider execution for a task.
type TaskAttempt struct {
	ID             string         `json:"id"`
	TaskID         string         `json:"task_id"`
	RunID          string         `json:"run_id"`
	AttemptNo      int            `json:"attempt_no"`
	ProviderName   string         `json:"provider_name"`
	AgentID        string         `json:"agent_id,omitempty"`
	Status         AttemptStatus  `json:"status"`
	InputPayload   map[string]any `json:"input_payload,omitempty"`
	OutputPayload  map[string]any `json:"output_payload,omitempty"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	Retryable      bool           `json:"retryable"`
	StartedAt      time.Time      `json:"started_at"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	IdempotencyKey string         `json:"idempotency_key"`
}

// RunEvent is append-only and intended for UI replay, audit, and recovery.
type RunEvent struct {
	ID        string         `json:"id"`
	RunID     string         `json:"run_id"`
	TaskID    string         `json:"task_id,omitempty"`
	Seq       int64          `json:"seq"`
	Type      EventType      `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// AgentDescriptor is the capability input seen by the planner.
type AgentDescriptor struct {
	ID           string         `json:"id"`
	ProviderName string         `json:"provider_name,omitempty"`
	Name         string         `json:"name,omitempty"`
	Capabilities []string       `json:"capabilities,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Plan is planner output before tasks are persisted.
type Plan struct {
	PlannerVersion string      `json:"planner_version"`
	Tasks          []TaskDraft `json:"tasks"`
}

// TaskDraft is a planned task. ClientKey is used to express dependencies before DB IDs exist.
type TaskDraft struct {
	ClientKey       string         `json:"client_key"` //nolint:gosec // JSON field name
	ParentClientKey string         `json:"parent_client_key,omitempty"`
	Title           string         `json:"title"`
	Description     string         `json:"description"`
	AssignedAgentID string         `json:"assigned_agent_id,omitempty"`
	ProviderName    string         `json:"provider_name,omitempty"`
	Priority        int            `json:"priority"`
	Timeout         time.Duration  `json:"timeout"`
	MaxRetries      int            `json:"max_retries"`
	DependsOn       []string       `json:"depends_on,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// RunSnapshot is a consistent read model returned by service methods.
type RunSnapshot struct {
	Run          Run              `json:"run"`
	Tasks        []Task           `json:"tasks"`
	Dependencies []TaskDependency `json:"dependencies"`
	Attempts     []TaskAttempt    `json:"attempts,omitempty"`
}

// StartRunInput starts a new orchestration run.
type StartRunInput struct {
	RoomID           string            `json:"room_id"`
	TriggerMessageID string            `json:"trigger_message_id,omitempty"`
	Objective        string            `json:"objective"`
	CreatedBy        string            `json:"created_by,omitempty"`
	Agents           []AgentDescriptor `json:"agents,omitempty"`
	Metadata         map[string]any    `json:"metadata,omitempty"`
	AutoDispatch     bool              `json:"auto_dispatch"`
}

// ExecuteTaskRequest is sent to an AgentProvider.
type ExecuteTaskRequest struct {
	Run       Run               `json:"run"`
	Task      Task              `json:"task"`
	AttemptNo int               `json:"attempt_no"`
	Context   map[string]any    `json:"context,omitempty"`
	Upstream  []TaskAttempt     `json:"upstream,omitempty"`
	Agents    []AgentDescriptor `json:"agents,omitempty"`
}

// ExecuteTaskResult is returned by an AgentProvider.
type ExecuteTaskResult struct {
	Output    map[string]any `json:"output,omitempty"`
	Summary   string         `json:"summary,omitempty"`
	Retryable bool           `json:"retryable"`
}
