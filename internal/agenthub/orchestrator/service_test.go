package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type flakyProvider struct {
	calls int32
}

func (*flakyProvider) Name() string           { return "flaky" }
func (*flakyProvider) Capabilities() []string { return []string{"code", "test"} }
func (p *flakyProvider) Execute(_ context.Context, req ExecuteTaskRequest) (ExecuteTaskResult, error) {
	call := atomic.AddInt32(&p.calls, 1)
	if req.Task.Title == "one" && call == 1 {
		return ExecuteTaskResult{Retryable: true}, errors.New("temporary failure")
	}
	return ExecuteTaskResult{Output: map[string]any{"ok": true, "task": req.Task.Title}}, nil
}

type fixedPlanner struct {
	plan Plan
}

func (p fixedPlanner) Plan(context.Context, PlanInput) (Plan, error) { return p.plan, nil }

func TestRulePlannerBuildsDAG(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanInput{
		RoomID:    "room-1",
		Objective: "build app",
		Agents: []AgentDescriptor{
			{ID: "codex", ProviderName: "codex", Capabilities: []string{"backend", "code"}},
			{ID: "claude", ProviderName: "claude", Capabilities: []string{"frontend", "ui"}},
		},
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(plan.Tasks) != 5 {
		t.Fatalf("expected 5 tasks, got %d", len(plan.Tasks))
	}
	if plan.Tasks[1].DependsOn[0] != "plan" || plan.Tasks[3].DependsOn[0] != "backend" {
		t.Fatalf("unexpected dependencies: %#v", plan.Tasks)
	}
}

func TestRulePlannerSingleAgentBuildsDirectTask(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanInput{
		RoomID:    "room-1",
		Objective: "用一句话说明测试是否真实执行",
		Agents: []AgentDescriptor{
			{ID: "bot:ds", ProviderName: "memoh", Name: "ds", Capabilities: []string{"chat", "plan"}},
		},
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(plan.Tasks) != 1 {
		t.Fatalf("expected 1 direct task, got %d", len(plan.Tasks))
	}
	task := plan.Tasks[0]
	if task.ClientKey != "direct-1" || task.AssignedAgentID != "bot:ds" || task.ProviderName != "memoh" {
		t.Fatalf("unexpected direct task: %#v", task)
	}
	if task.Description != "用一句话说明测试是否真实执行" {
		t.Fatalf("direct task should preserve objective, got %q", task.Description)
	}
	if task.Timeout != 10*time.Minute {
		t.Fatalf("default direct task timeout = %s, want 10m", task.Timeout)
	}
	if task.MaxRetries != 1 {
		t.Fatalf("default direct task max retries = %d, want 1", task.MaxRetries)
	}
}

func TestRulePlannerMentionedAgentsBuildDirectTasks(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanInput{
		RoomID:    "room-1",
		Objective: "@ds 做 plan，@cx 实现，@cc 验证",
		Agents: []AgentDescriptor{
			{ID: "bot:ds-id", ProviderName: "memoh", Name: "ds"},
			{ID: "bot:codex-id", ProviderName: "codex", Name: "cx"},
			{ID: "bot:claude-id", ProviderName: "claudecode", Name: "cc"},
		},
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(plan.Tasks) != 3 {
		t.Fatalf("expected 3 direct tasks, got %d", len(plan.Tasks))
	}
	wantProviders := []string{"memoh", "codex", "claudecode"}
	for i, want := range wantProviders {
		if plan.Tasks[i].ProviderName != want {
			t.Fatalf("task %d provider = %q, want %q; tasks=%#v", i, plan.Tasks[i].ProviderName, want, plan.Tasks)
		}
	}
	// Mentioned agents run independently in parallel — no dependency chain — so a
	// single agent failing/timing out can't cascade into a whole-run failure.
	for i, task := range plan.Tasks {
		if len(task.DependsOn) != 0 {
			t.Fatalf("task %d should have no dependencies (parallel), got %#v", i, task.DependsOn)
		}
	}
}

func TestRulePlannerMentionDoesNotMatchPrefixAgentName(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanInput{
		RoomID:    "room-1",
		Objective: "请 @ds 做一个页面，让其他 agent review",
		Agents: []AgentDescriptor{
			{ID: "bot:ds-pro-id", ProviderName: "memoh", Name: "ds-pro"},
			{ID: "bot:ds-id", ProviderName: "memoh", Name: "ds"},
		},
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(plan.Tasks) != 1 {
		t.Fatalf("expected only @ds to match one task, got %d: %#v", len(plan.Tasks), plan.Tasks)
	}
	if plan.Tasks[0].AssignedAgentID != "bot:ds-id" {
		t.Fatalf("@ds matched wrong agent: %#v", plan.Tasks[0])
	}
}

func TestServiceCompletesRunWithNoopProvider(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, fixedPlanner{plan: Plan{PlannerVersion: "test", Tasks: []TaskDraft{
		{ClientKey: "one", Title: "one", Description: "first", ProviderName: "noop", MaxRetries: 0, Timeout: time.Second},
		{ClientKey: "two", Title: "two", Description: "second", ProviderName: "noop", DependsOn: []string{"one"}, MaxRetries: 0, Timeout: time.Second},
	}}}, NewProviderRegistry(NoopProvider{}), nil, Config{})

	snap, err := svc.StartRun(context.Background(), StartRunInput{RoomID: "room-1", Objective: "ship", AutoDispatch: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if snap.Run.Status != RunStatusCompleted {
		t.Fatalf("expected completed run, got %s", snap.Run.Status)
	}
	if len(snap.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(snap.Attempts))
	}
}

// TestServiceRetriesRetryableFailure: a retryable failure is NOT hot-retried
// in the same reconcile pass — the retry waits out the exponential backoff
// (anchored on the failed attempt's FinishedAt) and a later reconcile
// dispatches it to success.
func TestServiceRetriesRetryableFailure(t *testing.T) {
	store := NewMemoryStore()
	provider := &flakyProvider{}
	svc := NewService(store, fixedPlanner{plan: Plan{PlannerVersion: "test", Tasks: []TaskDraft{
		{ClientKey: "one", Title: "one", Description: "first", ProviderName: "flaky", MaxRetries: 1, Timeout: time.Second},
	}}}, NewProviderRegistry(provider), nil, Config{})

	now := time.Now().UTC()
	svc.clock = func() time.Time { return now }

	snap, err := svc.StartRun(context.Background(), StartRunInput{RoomID: "room-1", Objective: "ship", AutoDispatch: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if snap.Run.Status == RunStatusCompleted {
		t.Fatal("retry must be deferred by backoff, not completed in the same pass")
	}
	if len(snap.Attempts) != 1 {
		t.Fatalf("expected 1 attempt before backoff elapses, got %d", len(snap.Attempts))
	}

	// Still inside the backoff window: reconcile must not redispatch.
	now = now.Add(retryBackoff(1) / 2)
	snap, err = svc.ReconcileRun(context.Background(), snap.Run.ID)
	if err != nil {
		t.Fatalf("ReconcileRun returned error: %v", err)
	}
	if len(snap.Attempts) != 1 {
		t.Fatalf("expected retry still deferred mid-backoff, got %d attempts", len(snap.Attempts))
	}

	// Past the backoff window: the retry dispatches and succeeds.
	now = now.Add(retryBackoff(1))
	snap, err = svc.ReconcileRun(context.Background(), snap.Run.ID)
	if err != nil {
		t.Fatalf("ReconcileRun returned error: %v", err)
	}
	if snap.Run.Status != RunStatusCompleted {
		t.Fatalf("expected completed run after backoff elapsed, got %s", snap.Run.Status)
	}
	if len(snap.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(snap.Attempts))
	}
}

func TestRetryBackoffSchedule(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 0, want: 0},
		{attempts: 1, want: 5 * time.Second},
		{attempts: 2, want: 10 * time.Second},
		{attempts: 3, want: 20 * time.Second},
		{attempts: 5, want: 60 * time.Second}, // capped
		{attempts: 50, want: 60 * time.Second},
	}
	for _, tc := range cases {
		if got := retryBackoff(tc.attempts); got != tc.want {
			t.Fatalf("retryBackoff(%d) = %s, want %s", tc.attempts, got, tc.want)
		}
	}
}

type failProvider struct{}

func (failProvider) Name() string           { return "fail" }
func (failProvider) Capabilities() []string { return []string{"code"} }
func (failProvider) Execute(_ context.Context, _ ExecuteTaskRequest) (ExecuteTaskResult, error) {
	return ExecuteTaskResult{Retryable: false}, errors.New("permanent failure")
}

func TestDependencyFailureBlocksDownstream(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, fixedPlanner{plan: Plan{PlannerVersion: "test", Tasks: []TaskDraft{
		{ClientKey: "a", Title: "a", Description: "first", ProviderName: "fail", MaxRetries: 0, Timeout: time.Second},
		{ClientKey: "b", Title: "b", Description: "second", ProviderName: "noop", DependsOn: []string{"a"}, MaxRetries: 0, Timeout: time.Second},
	}}}, NewProviderRegistry(failProvider{}, NoopProvider{}), nil, Config{})

	snap, err := svc.StartRun(context.Background(), StartRunInput{RoomID: "room-1", Objective: "ship", AutoDispatch: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if snap.Run.Status != RunStatusFailed {
		t.Fatalf("expected failed run, got %s", snap.Run.Status)
	}
	taskB := findTask(snap.Tasks, "b")
	if taskB == nil {
		t.Fatal("task b not found")
	}
	if taskB.Status != TaskStatusBlocked {
		t.Fatalf("expected task b blocked, got %s", taskB.Status)
	}
}

func TestCancelRunCancelsNonTerminalTasks(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, fixedPlanner{plan: Plan{PlannerVersion: "test", Tasks: []TaskDraft{
		{ClientKey: "a", Title: "a", Description: "first", ProviderName: "noop", MaxRetries: 0, Timeout: time.Second},
		{ClientKey: "b", Title: "b", Description: "second", ProviderName: "noop", DependsOn: []string{"a"}, MaxRetries: 0, Timeout: time.Second},
	}}}, NewProviderRegistry(NoopProvider{}), nil, Config{})

	snap, err := svc.StartRun(context.Background(), StartRunInput{RoomID: "room-1", Objective: "ship", AutoDispatch: false})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	snap, err = svc.CancelRun(context.Background(), snap.Run.ID)
	if err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}
	if snap.Run.Status != RunStatusCancelled {
		t.Fatalf("expected cancelled run, got %s", snap.Run.Status)
	}
	for _, task := range snap.Tasks {
		if task.Status != TaskStatusCancelled {
			t.Fatalf("expected task %s cancelled, got %s", task.Title, task.Status)
		}
	}
}

func TestTimeoutMarksTaskFailed(t *testing.T) {
	store := NewMemoryStore()
	slowProvider := &slowProvider{delay: 200 * time.Millisecond}
	svc := NewService(store, fixedPlanner{plan: Plan{PlannerVersion: "test", Tasks: []TaskDraft{
		{ClientKey: "a", Title: "a", Description: "slow task", ProviderName: "slow", MaxRetries: 0, Timeout: 50 * time.Millisecond},
	}}}, NewProviderRegistry(slowProvider), nil, Config{})

	snap, err := svc.StartRun(context.Background(), StartRunInput{RoomID: "room-1", Objective: "ship", AutoDispatch: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if snap.Run.Status != RunStatusFailed {
		t.Fatalf("expected failed run after timeout, got %s", snap.Run.Status)
	}
	if len(snap.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(snap.Attempts))
	}
	if snap.Attempts[0].Status != AttemptStatusTimedOut {
		t.Fatalf("expected timed_out attempt, got %s", snap.Attempts[0].Status)
	}
}

func TestAsyncReconcileFailsInterruptedRunningTaskAfterRestart(t *testing.T) {
	store := NewMemoryStore()
	startedAt := time.Now().UTC()
	run, err := store.CreateRun(context.Background(), Run{
		ID:             "run-restart",
		RoomID:         "room-1",
		Objective:      "ship",
		Status:         RunStatusCollecting,
		PlannerVersion: "test",
		CreatedAt:      startedAt.Add(-time.Minute),
		UpdatedAt:      startedAt.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	tasks, _, err := store.CreateTasks(context.Background(), run.ID, []TaskDraft{
		{ClientKey: "a", Title: "a", Description: "slow task", ProviderName: "slow", MaxRetries: 0, Timeout: time.Hour},
	})
	if err != nil {
		t.Fatalf("CreateTasks returned error: %v", err)
	}
	task, err := store.UpdateTaskStatus(context.Background(), tasks[0].ID, TaskStatusReady)
	if err != nil {
		t.Fatalf("UpdateTaskStatus ready returned error: %v", err)
	}
	task, err = store.UpdateTaskStatus(context.Background(), task.ID, TaskStatusRunning)
	if err != nil {
		t.Fatalf("UpdateTaskStatus running returned error: %v", err)
	}
	task, err = store.IncrementTaskAttempt(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("IncrementTaskAttempt returned error: %v", err)
	}
	_, err = store.CreateAttempt(context.Background(), TaskAttempt{
		ID:           "attempt-before-restart",
		TaskID:       task.ID,
		RunID:        run.ID,
		AttemptNo:    task.AttemptCount,
		ProviderName: "slow",
		AgentID:      task.AssignedAgentID,
		Status:       AttemptStatusRunning,
		StartedAt:    startedAt.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateAttempt returned error: %v", err)
	}

	svc := NewService(store, fixedPlanner{}, NewProviderRegistry(&slowProvider{delay: time.Hour}), nil, Config{DispatchAsync: true})
	svc.startedAt = startedAt

	snap, err := svc.ReconcileRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ReconcileRun returned error: %v", err)
	}
	if snap.Run.Status != RunStatusFailed {
		t.Fatalf("expected failed run after interrupted executor recovery, got %s", snap.Run.Status)
	}
	if len(snap.Attempts) != 1 || snap.Attempts[0].Status != AttemptStatusTimedOut {
		t.Fatalf("expected one timed_out attempt, got %#v", snap.Attempts)
	}
}

type slowProvider struct {
	delay time.Duration
}

func (*slowProvider) Name() string           { return "slow" }
func (*slowProvider) Capabilities() []string { return []string{"code"} }
func (p *slowProvider) Execute(ctx context.Context, _ ExecuteTaskRequest) (ExecuteTaskResult, error) {
	select {
	case <-time.After(p.delay):
		return ExecuteTaskResult{Output: map[string]any{"ok": true}}, nil
	case <-ctx.Done():
		return ExecuteTaskResult{}, ctx.Err()
	}
}

func findTask(tasks []Task, title string) *Task {
	for i := range tasks {
		if tasks[i].Title == title {
			return &tasks[i]
		}
	}
	return nil
}
