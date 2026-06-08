package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	MaxParallelPerRun   int
	MaxParallelPerAgent int
	DefaultTaskTimeout  time.Duration
	DispatchAsync       bool
}

type Service struct {
	store    Store
	planner  Planner
	registry *ProviderRegistry
	config   Config
	logger   *slog.Logger
	clock    func() time.Time
}

func NewService(store Store, planner Planner, registry *ProviderRegistry, logger *slog.Logger, cfg Config) *Service {
	if planner == nil {
		planner = NewRulePlanner()
	}
	if registry == nil {
		registry = NewProviderRegistry(NoopProvider{})
	}
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.MaxParallelPerRun <= 0 {
		cfg.MaxParallelPerRun = 3
	}
	if cfg.MaxParallelPerAgent <= 0 {
		cfg.MaxParallelPerAgent = 1
	}
	if cfg.DefaultTaskTimeout <= 0 {
		cfg.DefaultTaskTimeout = 10 * time.Minute
	}
	return &Service{
		store:    store,
		planner:  planner,
		registry: registry,
		config:   cfg,
		logger:   logger.With(slog.String("component", "agenthub_orchestrator")),
		clock:    time.Now,
	}
}

func (s *Service) StartRun(ctx context.Context, input StartRunInput) (RunSnapshot, error) {
	if s == nil || s.store == nil {
		return RunSnapshot{}, ErrInvalidInput
	}
	roomID := strings.TrimSpace(input.RoomID)
	objective := strings.TrimSpace(input.Objective)
	if roomID == "" || objective == "" {
		return RunSnapshot{}, ErrInvalidInput
	}

	plan, err := s.planner.Plan(ctx, PlanInput{RoomID: roomID, Objective: objective, Agents: input.Agents, Metadata: input.Metadata})
	if err != nil {
		return RunSnapshot{}, err
	}
	if err := validatePlan(plan); err != nil {
		return RunSnapshot{}, err
	}

	now := s.now()
	run, err := s.store.CreateRun(ctx, Run{
		ID:               uuid.NewString(),
		RoomID:           roomID,
		TriggerMessageID: strings.TrimSpace(input.TriggerMessageID),
		Objective:        objective,
		Status:           RunStatusPlanning,
		PlannerVersion:   firstNonEmpty(plan.PlannerVersion, RulePlannerVersion),
		CreatedBy:        strings.TrimSpace(input.CreatedBy),
		Metadata:         cloneMap(input.Metadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		return RunSnapshot{}, err
	}
	_, _ = s.appendEvent(ctx, run.ID, "", EventRunCreated, map[string]any{"objective": objective})

	tasks, deps, err := s.store.CreateTasks(ctx, run.ID, plan.Tasks)
	if err != nil {
		_, _ = s.transitionRun(ctx, run, RunStatusFailed)
		return RunSnapshot{}, err
	}
	for _, task := range tasks {
		_, _ = s.appendEvent(ctx, run.ID, task.ID, EventTaskCreated, map[string]any{"title": task.Title, "assigned_agent_id": task.AssignedAgentID, "provider_name": task.ProviderName})
	}
	_, _ = s.appendEvent(ctx, run.ID, "", EventRunPlanned, map[string]any{"tasks": len(tasks), "dependencies": len(deps), "planner_version": run.PlannerVersion})

	run, err = s.transitionRun(ctx, run, RunStatusDispatching)
	if err != nil {
		return RunSnapshot{}, err
	}
	if _, err := s.markRunnableTasks(ctx, run.ID); err != nil {
		return RunSnapshot{}, err
	}
	if input.AutoDispatch {
		if _, err := s.ReconcileRun(ctx, run.ID); err != nil {
			return RunSnapshot{}, err
		}
	}
	return s.GetSnapshot(ctx, run.ID)
}

func (s *Service) GetSnapshot(ctx context.Context, runID string) (RunSnapshot, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return RunSnapshot{}, err
	}
	tasks, err := s.store.ListTasks(ctx, run.ID)
	if err != nil {
		return RunSnapshot{}, err
	}
	deps, err := s.store.ListDependencies(ctx, run.ID)
	if err != nil {
		return RunSnapshot{}, err
	}
	attempts, err := s.store.ListAttempts(ctx, run.ID)
	if err != nil {
		return RunSnapshot{}, err
	}
	return RunSnapshot{Run: run, Tasks: tasks, Dependencies: deps, Attempts: attempts}, nil
}

// LatestSnapshotByRoom returns a full snapshot of the room's most recent run,
// or ErrNotFound if the room has no runs yet.
func (s *Service) LatestSnapshotByRoom(ctx context.Context, roomID string) (RunSnapshot, error) {
	run, err := s.store.GetLatestRunByRoom(ctx, strings.TrimSpace(roomID))
	if err != nil {
		return RunSnapshot{}, err
	}
	return s.GetSnapshot(ctx, run.ID)
}

func (s *Service) ReconcileRun(ctx context.Context, runID string) (RunSnapshot, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return RunSnapshot{}, err
	}
	if isTerminalRunStatus(run.Status) {
		return s.GetSnapshot(ctx, run.ID)
	}

	// Bounded loop: in sync mode, completing a task may unblock downstream
	// tasks that should be dispatched in the same reconcile call. The loop
	// replaces the previous recursive ReconcileRun call inside
	// completeTaskAttempt, keeping the call stack flat.
	const maxIterations = 100
	for i := 0; i < maxIterations; i++ {
		if _, err := s.failTimedOutTasks(ctx, run); err != nil {
			return RunSnapshot{}, err
		}
		if _, err := s.markRunnableTasks(ctx, run.ID); err != nil {
			return RunSnapshot{}, err
		}
		run, err = s.recomputeRunStatus(ctx, run)
		if err != nil {
			return RunSnapshot{}, err
		}
		if isTerminalRunStatus(run.Status) {
			break
		}
		if err := s.dispatchReadyTasks(ctx, run); err != nil {
			if errors.Is(err, ErrNoExecutableTasks) {
				break
			}
			return RunSnapshot{}, err
		}
		// Async mode: tasks run in goroutines that call ReconcileRun on
		// completion, so there is no inline progress to loop on.
		if s.config.DispatchAsync {
			break
		}
		// Sync mode: dispatched tasks already executed inline. Re-read
		// the run to pick up state changes and loop to advance the DAG.
		run, err = s.store.GetRun(ctx, run.ID)
		if err != nil {
			return RunSnapshot{}, err
		}
	}
	return s.GetSnapshot(ctx, run.ID)
}

func (s *Service) ListActiveRuns(ctx context.Context) ([]Run, error) {
	return s.store.ListRunsByStatus(ctx, RunStatusPlanning, RunStatusDispatching, RunStatusCollecting)
}

func (s *Service) ReconcileActiveRuns(ctx context.Context) ([]RunSnapshot, error) {
	runs, err := s.store.ListRunsByStatus(ctx, RunStatusPlanning, RunStatusDispatching, RunStatusCollecting)
	if err != nil {
		return nil, err
	}
	snapshots := make([]RunSnapshot, 0, len(runs))
	for _, run := range runs {
		snapshot, err := s.ReconcileRun(ctx, run.ID)
		if err != nil {
			return snapshots, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func (s *Service) ListEvents(ctx context.Context, runID string, afterSeq int64, limit int32) ([]RunEvent, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	return s.store.ListEvents(ctx, run.ID, afterSeq, limit)
}

func (s *Service) CancelRun(ctx context.Context, runID string) (RunSnapshot, error) {
	run, err := s.store.GetRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return RunSnapshot{}, err
	}
	if isTerminalRunStatus(run.Status) {
		return s.GetSnapshot(ctx, run.ID)
	}
	tasks, err := s.store.ListTasks(ctx, run.ID)
	if err != nil {
		return RunSnapshot{}, err
	}
	for _, task := range tasks {
		if task.Status == TaskStatusSucceeded || task.Status == TaskStatusCancelled {
			continue
		}
		if !canTransitionTask(task.Status, TaskStatusCancelled) {
			continue
		}
		updated, err := s.store.UpdateTaskStatus(ctx, task.ID, TaskStatusCancelled)
		if err != nil {
			return RunSnapshot{}, err
		}
		_, _ = s.appendEvent(ctx, run.ID, updated.ID, EventTaskCancelled, nil)
	}
	if _, err := s.transitionRun(ctx, run, RunStatusCancelled); err != nil {
		return RunSnapshot{}, err
	}
	return s.GetSnapshot(ctx, run.ID)
}

func (s *Service) dispatchReadyTasks(ctx context.Context, run Run) error {
	tasks, err := s.store.ListTasks(ctx, run.ID)
	if err != nil {
		return err
	}
	ready := make([]Task, 0)
	runningPerAgent := map[string]int{}
	runningTotal := 0
	for _, task := range tasks {
		switch task.Status {
		case TaskStatusReady:
			ready = append(ready, task)
		case TaskStatusRunning:
			runningTotal++
			runningPerAgent[agentKey(task)]++
		}
	}

	s.logger.Info("dispatchReadyTasks checking ready tasks",
		slog.String("run_id", run.ID),
		slog.Int("total_tasks", len(tasks)),
		slog.Int("ready_tasks", len(ready)),
		slog.Int("running_total", runningTotal),
	)

	if len(ready) == 0 {
		return ErrNoExecutableTasks
	}
	sort.SliceStable(ready, func(i, j int) bool {
		if ready[i].Priority != ready[j].Priority {
			return ready[i].Priority > ready[j].Priority
		}
		return ready[i].CreatedAt.Before(ready[j].CreatedAt)
	})

	capacity := s.config.MaxParallelPerRun - runningTotal
	if capacity <= 0 {
		return ErrNoExecutableTasks
	}
	dispatched := 0
	for _, task := range ready {
		if dispatched >= capacity {
			break
		}
		key := agentKey(task)
		if runningPerAgent[key] >= s.config.MaxParallelPerAgent {
			continue
		}
		provider, ok := s.registry.Resolve(task.ProviderName)
		if !ok {
			provider, ok = s.registry.Resolve("")
		}
		if !ok {
			s.logger.Error("provider not found in registry",
				slog.String("run_id", run.ID),
				slog.String("task_id", task.ID),
				slog.String("provider_name", task.ProviderName),
			)
			return ErrProviderNotFound
		}
		runningTask, err := s.store.UpdateTaskStatus(ctx, task.ID, TaskStatusRunning)
		if err != nil {
			if errors.Is(err, ErrInvalidTransition) {
				continue
			}
			s.logger.Error("failed to update task status to running",
				slog.String("run_id", run.ID),
				slog.String("task_id", task.ID),
				slog.Any("error", err),
			)
			return err
		}
		runningTask, err = s.store.IncrementTaskAttempt(ctx, runningTask.ID)
		if err != nil {
			s.logger.Error("failed to increment task attempt",
				slog.String("run_id", run.ID),
				slog.String("task_id", task.ID),
				slog.Any("error", err),
			)
			return err
		}
		attempt, err := s.createAttempt(ctx, run, runningTask, provider)
		if err != nil {
			s.logger.Error("failed to create task attempt",
				slog.String("run_id", run.ID),
				slog.String("task_id", task.ID),
				slog.Any("error", err),
			)
			return err
		}
		_, _ = s.appendEvent(ctx, run.ID, runningTask.ID, EventTaskDispatched, map[string]any{"attempt_id": attempt.ID, "attempt_no": attempt.AttemptNo, "provider_name": attempt.ProviderName, "agent_id": attempt.AgentID})

		s.logger.Info("dispatching task to provider",
			slog.String("run_id", run.ID),
			slog.String("task_id", task.ID),
			slog.String("provider", provider.Name()),
			slog.Int("attempt_no", attempt.AttemptNo),
		)

		dispatched++
		runningPerAgent[key]++
		if s.config.DispatchAsync {
			go func(r Run, t Task, a TaskAttempt, p AgentProvider) {
				s.executeAttempt(context.WithoutCancel(ctx), r, t, a, p)
				_, _ = s.ReconcileRun(context.WithoutCancel(ctx), r.ID)
			}(run, runningTask, attempt, provider)
		} else {
			s.executeAttempt(ctx, run, runningTask, attempt, provider)
		}
	}
	if dispatched == 0 {
		return ErrNoExecutableTasks
	}
	return nil
}

func (s *Service) createAttempt(ctx context.Context, run Run, task Task, provider AgentProvider) (TaskAttempt, error) {
	attemptNo := task.AttemptCount
	if attemptNo <= 0 {
		attemptNo = 1
	}
	key := idempotencyKey(run.ID, task.ID, attemptNo, task.Description)
	return s.store.CreateAttempt(ctx, TaskAttempt{
		ID:             uuid.NewString(),
		TaskID:         task.ID,
		RunID:          run.ID,
		AttemptNo:      attemptNo,
		ProviderName:   provider.Name(),
		AgentID:        task.AssignedAgentID,
		Status:         AttemptStatusRunning,
		InputPayload:   map[string]any{"title": task.Title, "description": task.Description},
		StartedAt:      s.now(),
		IdempotencyKey: key,
	})
}

func (s *Service) executeAttempt(ctx context.Context, run Run, task Task, attempt TaskAttempt, provider AgentProvider) {
	timeout := task.Timeout
	if timeout <= 0 {
		timeout = s.config.DefaultTaskTimeout
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s.logger.Info("executing task attempt",
		slog.String("run_id", run.ID),
		slog.String("task_id", task.ID),
		slog.String("attempt_id", attempt.ID),
		slog.String("provider", provider.Name()),
	)

	attempts, _ := s.store.ListAttempts(ctx, run.ID)
	deps, _ := s.store.ListDependencies(ctx, run.ID)
	upstream := successfulAttemptsForDependencies(task, deps, attempts)
	result, err := provider.Execute(execCtx, ExecuteTaskRequest{Run: run, Task: task, AttemptNo: attempt.AttemptNo, Context: run.Metadata, Upstream: upstream})
	if execCtx.Err() == context.DeadlineExceeded {
		s.logger.Warn("task attempt timed out",
			slog.String("run_id", run.ID),
			slog.String("task_id", task.ID),
			slog.String("attempt_id", attempt.ID),
			slog.Duration("timeout", timeout),
		)
		_ = s.completeTaskAttempt(context.WithoutCancel(ctx), run, task, attempt, AttemptStatusTimedOut, nil, "task timed out", true)
		return
	}
	if err != nil {
		s.logger.Error("task attempt execution error",
			slog.String("run_id", run.ID),
			slog.String("task_id", task.ID),
			slog.String("attempt_id", attempt.ID),
			slog.Any("error", err),
			slog.Bool("retryable", result.Retryable),
		)
		_ = s.completeTaskAttempt(context.WithoutCancel(ctx), run, task, attempt, AttemptStatusFailed, nil, err.Error(), result.Retryable)
		return
	}
	s.logger.Info("task attempt completed successfully",
		slog.String("run_id", run.ID),
		slog.String("task_id", task.ID),
		slog.String("attempt_id", attempt.ID),
	)
	output := cloneMap(result.Output)
	if result.Summary != "" {
		output["summary"] = result.Summary
	}
	_ = s.completeTaskAttempt(context.WithoutCancel(ctx), run, task, attempt, AttemptStatusSucceeded, output, "", false)
}

func (s *Service) completeTaskAttempt(ctx context.Context, run Run, task Task, attempt TaskAttempt, status AttemptStatus, output map[string]any, errorMessage string, retryable bool) error {
	_, err := s.store.CompleteAttempt(ctx, attempt.ID, status, output, errorMessage, retryable)
	if err != nil {
		return err
	}
	if status == AttemptStatusSucceeded {
		updated, err := s.store.UpdateTaskStatus(ctx, task.ID, TaskStatusSucceeded)
		if err != nil {
			return err
		}
		_, _ = s.appendEvent(ctx, run.ID, updated.ID, EventTaskSucceeded, map[string]any{"attempt_id": attempt.ID, "output": output})
	} else {
		// MaxRetries is the number of *extra* attempts after the first.
		// AttemptCount is already incremented before dispatch, so
		// AttemptCount==1 on first run; MaxRetries==1 allows one retry.
		if retryable && task.AttemptCount <= task.MaxRetries {
			failed, err := s.store.UpdateTaskStatus(ctx, task.ID, TaskStatusFailed)
			if err != nil {
				return err
			}
			_, _ = s.appendEvent(ctx, run.ID, failed.ID, EventTaskFailed, map[string]any{"attempt_id": attempt.ID, "error": errorMessage, "retryable": true})
			updated, err := s.store.UpdateTaskStatus(ctx, task.ID, TaskStatusReady)
			if err != nil {
				return err
			}
			_, _ = s.appendEvent(ctx, run.ID, updated.ID, EventTaskRetried, map[string]any{"attempt_id": attempt.ID, "error": errorMessage, "attempt_count": task.AttemptCount, "max_retries": task.MaxRetries})
		} else {
			updated, err := s.store.UpdateTaskStatus(ctx, task.ID, TaskStatusFailed)
			if err != nil {
				return err
			}
			eventType := EventTaskFailed
			if status == AttemptStatusTimedOut {
				eventType = EventTaskTimedOut
			}
			_, _ = s.appendEvent(ctx, run.ID, updated.ID, eventType, map[string]any{"attempt_id": attempt.ID, "error": errorMessage, "retryable": retryable})
		}
	}
	return nil
}

func (s *Service) markRunnableTasks(ctx context.Context, runID string) (int, error) {
	tasks, err := s.store.ListTasks(ctx, runID)
	if err != nil {
		return 0, err
	}
	deps, err := s.store.ListDependencies(ctx, runID)
	if err != nil {
		return 0, err
	}
	byID := make(map[string]Task, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
	}
	depsByTask := make(map[string][]TaskDependency)
	for _, dep := range deps {
		depsByTask[dep.TaskID] = append(depsByTask[dep.TaskID], dep)
	}
	changed := 0
	for _, task := range tasks {
		if task.Status != TaskStatusPending && task.Status != TaskStatusBlocked {
			continue
		}
		blocked := false
		ready := true
		for _, dep := range depsByTask[task.ID] {
			parent := byID[dep.DependsOnTaskID]
			if parent.Status == TaskStatusFailed || parent.Status == TaskStatusBlocked || parent.Status == TaskStatusCancelled {
				blocked = true
				break
			}
			if parent.Status != TaskStatusSucceeded {
				ready = false
			}
		}
		if blocked {
			if task.Status != TaskStatusBlocked {
				updated, err := s.store.UpdateTaskStatus(ctx, task.ID, TaskStatusBlocked)
				if err != nil {
					return changed, err
				}
				_, _ = s.appendEvent(ctx, runID, updated.ID, EventTaskBlocked, nil)
				changed++
			}
			continue
		}
		if ready {
			updated, err := s.store.UpdateTaskStatus(ctx, task.ID, TaskStatusReady)
			if err != nil {
				return changed, err
			}
			_, _ = s.appendEvent(ctx, runID, updated.ID, EventTaskReady, nil)
			changed++
		}
	}
	return changed, nil
}

func (s *Service) failTimedOutTasks(ctx context.Context, run Run) (int, error) {
	tasks, err := s.store.ListTasks(ctx, run.ID)
	if err != nil {
		return 0, err
	}
	attempts, err := s.store.ListAttempts(ctx, run.ID)
	if err != nil {
		return 0, err
	}
	latestByTask := make(map[string]TaskAttempt)
	for _, attempt := range attempts {
		if attempt.Status != AttemptStatusRunning {
			continue
		}
		if current, ok := latestByTask[attempt.TaskID]; !ok || attempt.AttemptNo > current.AttemptNo {
			latestByTask[attempt.TaskID] = attempt
		}
	}
	failed := 0
	for _, task := range tasks {
		if task.Status != TaskStatusRunning {
			continue
		}
		attempt, ok := latestByTask[task.ID]
		if !ok {
			continue
		}
		timeout := task.Timeout
		if timeout <= 0 {
			timeout = s.config.DefaultTaskTimeout
		}
		if s.now().Sub(attempt.StartedAt) < timeout {
			continue
		}
		if err := s.completeTaskAttempt(ctx, run, task, attempt, AttemptStatusTimedOut, nil, "task timed out", true); err != nil {
			return failed, err
		}
		failed++
	}
	return failed, nil
}

func (s *Service) recomputeRunStatus(ctx context.Context, run Run) (Run, error) {
	tasks, err := s.store.ListTasks(ctx, run.ID)
	if err != nil {
		return run, err
	}
	if len(tasks) == 0 {
		return s.transitionRun(ctx, run, RunStatusFailed)
	}
	allSucceeded := true
	anyRunning := false
	anyReadyOrPending := false
	anyFailedOrBlocked := false
	for _, task := range tasks {
		switch task.Status {
		case TaskStatusSucceeded:
		case TaskStatusRunning:
			allSucceeded = false
			anyRunning = true
		case TaskStatusReady, TaskStatusPending:
			allSucceeded = false
			anyReadyOrPending = true
		case TaskStatusFailed, TaskStatusBlocked, TaskStatusCancelled:
			allSucceeded = false
			anyFailedOrBlocked = true
		default:
			allSucceeded = false
		}
	}
	if allSucceeded {
		return s.transitionRun(ctx, run, RunStatusCompleted)
	}
	if anyFailedOrBlocked && !anyRunning && !anyReadyOrPending {
		return s.transitionRun(ctx, run, RunStatusFailed)
	}
	if anyRunning {
		return s.transitionRun(ctx, run, RunStatusCollecting)
	}
	return s.transitionRun(ctx, run, RunStatusDispatching)
}

func (s *Service) transitionRun(ctx context.Context, run Run, status RunStatus) (Run, error) {
	if !canTransitionRun(run.Status, status) {
		return run, ErrInvalidTransition
	}
	if run.Status == status {
		return run, nil
	}
	updated, err := s.store.UpdateRunStatus(ctx, run.ID, status)
	if err != nil {
		return run, err
	}
	_, _ = s.appendEvent(ctx, run.ID, "", EventRunStatusChanged, map[string]any{"from": run.Status, "to": status})
	return updated, nil
}

func (s *Service) appendEvent(ctx context.Context, runID, taskID string, eventType EventType, payload map[string]any) (RunEvent, error) {
	return s.store.AppendEvent(ctx, RunEvent{ID: uuid.NewString(), RunID: runID, TaskID: taskID, Type: eventType, Payload: cloneMap(payload), CreatedAt: s.now()})
}

func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock().UTC()
	}
	return time.Now().UTC()
}

func validatePlan(plan Plan) error {
	if len(plan.Tasks) == 0 {
		return ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(plan.Tasks))
	for _, task := range plan.Tasks {
		key := strings.TrimSpace(task.ClientKey)
		if key == "" || strings.TrimSpace(task.Title) == "" {
			return ErrInvalidInput
		}
		if _, ok := seen[key]; ok {
			return ErrDuplicateClientKey
		}
		seen[key] = struct{}{}
	}
	for _, task := range plan.Tasks {
		for _, dep := range task.DependsOn {
			if _, ok := seen[strings.TrimSpace(dep)]; !ok {
				return ErrInvalidInput
			}
		}
	}
	return nil
}

func idempotencyKey(runID, taskID string, attemptNo int, input string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%s", runID, taskID, attemptNo, input)))
	return hex.EncodeToString(sum[:])
}

func agentKey(task Task) string {
	if strings.TrimSpace(task.AssignedAgentID) != "" {
		return strings.TrimSpace(task.AssignedAgentID)
	}
	if strings.TrimSpace(task.ProviderName) != "" {
		return "provider:" + strings.TrimSpace(task.ProviderName)
	}
	return "default"
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		switch val := v.(type) {
		case map[string]any:
			out[k] = cloneMap(val)
		case []any:
			cp := make([]any, len(val))
			copy(cp, val)
			out[k] = cp
		default:
			out[k] = v
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func successfulAttemptsForDependencies(task Task, deps []TaskDependency, attempts []TaskAttempt) []TaskAttempt {
	allowed := make(map[string]struct{})
	for _, dep := range deps {
		if dep.TaskID == task.ID {
			allowed[dep.DependsOnTaskID] = struct{}{}
		}
	}
	out := make([]TaskAttempt, 0)
	for _, attempt := range attempts {
		if attempt.Status != AttemptStatusSucceeded {
			continue
		}
		if len(allowed) == 0 {
			continue
		}
		if _, ok := allowed[attempt.TaskID]; ok {
			out = append(out, attempt)
		}
	}
	return out
}
