package orchestrator

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore is a concurrency-safe Store implementation for tests and local
// development. Production should implement Store with sqlc-backed persistence.
type MemoryStore struct {
	mu            sync.RWMutex
	runs          map[string]Run
	tasks         map[string]Task
	tasksByRun    map[string][]string
	depsByRun     map[string][]TaskDependency
	attempts      map[string]TaskAttempt
	attemptsByRun map[string][]string
	eventsByRun   map[string][]RunEvent
	seqByRun      map[string]int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		runs:          make(map[string]Run),
		tasks:         make(map[string]Task),
		tasksByRun:    make(map[string][]string),
		depsByRun:     make(map[string][]TaskDependency),
		attempts:      make(map[string]TaskAttempt),
		attemptsByRun: make(map[string][]string),
		eventsByRun:   make(map[string][]RunEvent),
		seqByRun:      make(map[string]int64),
	}
}

func (s *MemoryStore) CreateRun(_ context.Context, run Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(run.ID) == "" {
		run.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = run.CreatedAt
	}
	s.runs[run.ID] = cloneRun(run)
	return cloneRun(run), nil
}

func (s *MemoryStore) GetRun(_ context.Context, runID string) (Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[strings.TrimSpace(runID)]
	if !ok {
		return Run{}, ErrNotFound
	}
	return cloneRun(run), nil
}

func (s *MemoryStore) UpdateRunStatus(_ context.Context, runID string, status RunStatus) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[strings.TrimSpace(runID)]
	if !ok {
		return Run{}, ErrNotFound
	}
	if !canTransitionRun(run.Status, status) {
		return Run{}, ErrInvalidTransition
	}
	run.Status = status
	run.UpdatedAt = time.Now().UTC()
	s.runs[run.ID] = run
	return cloneRun(run), nil
}

func (s *MemoryStore) ListRunsByStatus(_ context.Context, statuses ...RunStatus) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wanted := make(map[RunStatus]struct{}, len(statuses))
	for _, status := range statuses {
		wanted[status] = struct{}{}
	}
	out := make([]Run, 0)
	for _, run := range s.runs {
		if len(wanted) > 0 {
			if _, ok := wanted[run.Status]; !ok {
				continue
			}
		}
		out = append(out, cloneRun(run))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) GetLatestRunByRoom(_ context.Context, roomID string) (Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roomID = strings.TrimSpace(roomID)
	var latest Run
	found := false
	for _, run := range s.runs {
		if run.RoomID != roomID {
			continue
		}
		if !found ||
			run.UpdatedAt.After(latest.UpdatedAt) ||
			(run.UpdatedAt.Equal(latest.UpdatedAt) && run.CreatedAt.After(latest.CreatedAt)) {
			latest = run
			found = true
		}
	}
	if !found {
		return Run{}, ErrNotFound
	}
	return cloneRun(latest), nil
}

func (s *MemoryStore) CreateTasks(_ context.Context, runID string, drafts []TaskDraft) ([]Task, []TaskDependency, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[runID]; !ok {
		return nil, nil, ErrNotFound
	}
	clientToID := make(map[string]string, len(drafts))
	now := time.Now().UTC()
	tasks := make([]Task, 0, len(drafts))
	for _, draft := range drafts {
		key := strings.TrimSpace(draft.ClientKey)
		if key == "" {
			return nil, nil, ErrInvalidInput
		}
		if _, ok := clientToID[key]; ok {
			return nil, nil, ErrDuplicateClientKey
		}
		id := uuid.NewString()
		clientToID[key] = id
		status := TaskStatusPending
		if len(draft.DependsOn) == 0 {
			status = TaskStatusPending
		}
		task := Task{
			ID:              id,
			RunID:           runID,
			Title:           strings.TrimSpace(draft.Title),
			Description:     strings.TrimSpace(draft.Description),
			AssignedAgentID: strings.TrimSpace(draft.AssignedAgentID),
			ProviderName:    strings.TrimSpace(draft.ProviderName),
			Priority:        draft.Priority,
			Status:          status,
			Timeout:         draft.Timeout,
			MaxRetries:      draft.MaxRetries,
			Metadata:        cloneMap(draft.Metadata),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if strings.TrimSpace(draft.ParentClientKey) != "" {
			task.ParentTaskID = clientToID[strings.TrimSpace(draft.ParentClientKey)]
		}
		s.tasks[task.ID] = task
		s.tasksByRun[runID] = append(s.tasksByRun[runID], task.ID)
		tasks = append(tasks, cloneTask(task))
	}
	deps := make([]TaskDependency, 0)
	for _, draft := range drafts {
		taskID := clientToID[strings.TrimSpace(draft.ClientKey)]
		for _, depKey := range draft.DependsOn {
			depID := clientToID[strings.TrimSpace(depKey)]
			if depID == "" {
				return nil, nil, ErrInvalidInput
			}
			dep := TaskDependency{RunID: runID, TaskID: taskID, DependsOnTaskID: depID}
			s.depsByRun[runID] = append(s.depsByRun[runID], dep)
			deps = append(deps, dep)
		}
	}
	return tasks, deps, nil
}

func (s *MemoryStore) ListTasks(_ context.Context, runID string) ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.tasksByRun[strings.TrimSpace(runID)]
	out := make([]Task, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneTask(s.tasks[id]))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) UpdateTaskStatus(_ context.Context, taskID string, status TaskStatus) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[strings.TrimSpace(taskID)]
	if !ok {
		return Task{}, ErrNotFound
	}
	if !canTransitionTask(task.Status, status) {
		return Task{}, ErrInvalidTransition
	}
	task.Status = status
	task.UpdatedAt = time.Now().UTC()
	s.tasks[task.ID] = task
	return cloneTask(task), nil
}

func (s *MemoryStore) IncrementTaskAttempt(_ context.Context, taskID string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[strings.TrimSpace(taskID)]
	if !ok {
		return Task{}, ErrNotFound
	}
	task.AttemptCount++
	task.UpdatedAt = time.Now().UTC()
	s.tasks[task.ID] = task
	return cloneTask(task), nil
}

func (s *MemoryStore) ListDependencies(_ context.Context, runID string) ([]TaskDependency, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	deps := s.depsByRun[strings.TrimSpace(runID)]
	out := make([]TaskDependency, len(deps))
	copy(out, deps)
	return out, nil
}

func (s *MemoryStore) CreateAttempt(_ context.Context, attempt TaskAttempt) (TaskAttempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[attempt.TaskID]; !ok {
		return TaskAttempt{}, ErrNotFound
	}
	if strings.TrimSpace(attempt.ID) == "" {
		attempt.ID = uuid.NewString()
	}
	if attempt.StartedAt.IsZero() {
		attempt.StartedAt = time.Now().UTC()
	}
	s.attempts[attempt.ID] = cloneAttempt(attempt)
	s.attemptsByRun[attempt.RunID] = append(s.attemptsByRun[attempt.RunID], attempt.ID)
	return cloneAttempt(attempt), nil
}

func (s *MemoryStore) CompleteAttempt(_ context.Context, attemptID string, status AttemptStatus, output map[string]any, errorMessage string, retryable bool) (TaskAttempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, ok := s.attempts[strings.TrimSpace(attemptID)]
	if !ok {
		return TaskAttempt{}, ErrNotFound
	}
	attempt.Status = status
	attempt.OutputPayload = cloneMap(output)
	attempt.ErrorMessage = strings.TrimSpace(errorMessage)
	attempt.Retryable = retryable
	now := time.Now().UTC()
	attempt.FinishedAt = &now
	s.attempts[attempt.ID] = attempt
	return cloneAttempt(attempt), nil
}

func (s *MemoryStore) ListAttempts(_ context.Context, runID string) ([]TaskAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.attemptsByRun[strings.TrimSpace(runID)]
	out := make([]TaskAttempt, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneAttempt(s.attempts[id]))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].AttemptNo < out[j].AttemptNo
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out, nil
}

func (s *MemoryStore) AppendEvent(_ context.Context, event RunEvent) (RunEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[event.RunID]; !ok {
		return RunEvent{}, ErrNotFound
	}
	if strings.TrimSpace(event.ID) == "" {
		event.ID = uuid.NewString()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	s.seqByRun[event.RunID]++
	event.Seq = s.seqByRun[event.RunID]
	event.Payload = cloneMap(event.Payload)
	s.eventsByRun[event.RunID] = append(s.eventsByRun[event.RunID], event)
	return cloneEvent(event), nil
}

func (s *MemoryStore) ListEvents(_ context.Context, runID string, afterSeq int64, limit int32) ([]RunEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := s.eventsByRun[strings.TrimSpace(runID)]
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	out := make([]RunEvent, 0, len(events))
	for _, event := range events {
		if event.Seq <= afterSeq {
			continue
		}
		out = append(out, cloneEvent(event))
		if int32(len(out)) >= limit { //nolint:gosec // len is bounded by limit
			break
		}
	}
	return out, nil
}

func cloneRun(run Run) Run {
	run.Metadata = cloneMap(run.Metadata)
	return run
}

func cloneTask(task Task) Task {
	task.Metadata = cloneMap(task.Metadata)
	return task
}

func cloneAttempt(attempt TaskAttempt) TaskAttempt {
	attempt.InputPayload = cloneMap(attempt.InputPayload)
	attempt.OutputPayload = cloneMap(attempt.OutputPayload)
	if attempt.FinishedAt != nil {
		finished := *attempt.FinishedAt
		attempt.FinishedAt = &finished
	}
	return attempt
}

func cloneEvent(event RunEvent) RunEvent {
	event.Payload = cloneMap(event.Payload)
	return event
}
