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

func (p *flakyProvider) Name() string           { return "flaky" }
func (p *flakyProvider) Capabilities() []string { return []string{"code", "test"} }
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

func TestServiceRetriesRetryableFailure(t *testing.T) {
	store := NewMemoryStore()
	provider := &flakyProvider{}
	svc := NewService(store, fixedPlanner{plan: Plan{PlannerVersion: "test", Tasks: []TaskDraft{
		{ClientKey: "one", Title: "one", Description: "first", ProviderName: "flaky", MaxRetries: 1, Timeout: time.Second},
	}}}, NewProviderRegistry(provider), nil, Config{})

	snap, err := svc.StartRun(context.Background(), StartRunInput{RoomID: "room-1", Objective: "ship", AutoDispatch: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if snap.Run.Status != RunStatusCompleted {
		t.Fatalf("expected completed run after retry, got %s", snap.Run.Status)
	}
	if len(snap.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(snap.Attempts))
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

func TestGetLatestRunForRoomReturnsNewestRun(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, fixedPlanner{plan: Plan{PlannerVersion: "test", Tasks: []TaskDraft{
		{ClientKey: "a", Title: "a", Description: "first", ProviderName: "noop", MaxRetries: 0, Timeout: time.Second},
	}}}, NewProviderRegistry(NoopProvider{}), nil, Config{})

	first, err := svc.StartRun(context.Background(), StartRunInput{RoomID: "room-1", Objective: "ship api", AutoDispatch: true})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	second, err := svc.StartRun(context.Background(), StartRunInput{RoomID: "room-2", Objective: "ship web", AutoDispatch: true})
	if err != nil {
		t.Fatalf("second StartRun returned error: %v", err)
	}
	latest, err := svc.StartRun(context.Background(), StartRunInput{RoomID: "room-1", Objective: "ship tests", AutoDispatch: true})
	if err != nil {
		t.Fatalf("latest StartRun returned error: %v", err)
	}

	snap, err := svc.GetLatestRunForRoom(context.Background(), "room-1")
	if err != nil {
		t.Fatalf("GetLatestRunForRoom returned error: %v", err)
	}
	if snap.Run.ID != latest.Run.ID {
		t.Fatalf("expected latest run %s, got %s", latest.Run.ID, snap.Run.ID)
	}
	if snap.Run.Objective != "ship tests" {
		t.Fatalf("expected latest objective ship tests, got %s", snap.Run.Objective)
	}
	if snap.Run.ID == first.Run.ID || snap.Run.ID == second.Run.ID {
		t.Fatalf("expected newest room-1 run, got old run %s", snap.Run.ID)
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

type slowProvider struct {
	delay time.Duration
}

func (p *slowProvider) Name() string           { return "slow" }
func (p *slowProvider) Capabilities() []string { return []string{"code"} }
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
