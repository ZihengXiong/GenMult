package agenthub

import (
	"context"
	"strings"
	"testing"
	"time"

	orch "github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

func TestFriendlyAgentName(t *testing.T) {
	cases := []struct {
		in           string
		wantProvider string
		wantName     string
	}{
		{"claude-code", "claudecode", "Claude Code"},
		{"Claude", "claudecode", "Claude Code"},
		{"codex", "codex", "Codex"},
		{"orchestrator", "noop", "Orchestrator"},
		{"", "noop", "Orchestrator"},
		{"my-custom-bot", "noop", "my-custom-bot"},
	}
	for _, tc := range cases {
		gotProvider, gotName := friendlyAgentName(tc.in)
		if gotProvider != tc.wantProvider || gotName != tc.wantName {
			t.Errorf("friendlyAgentName(%q) = (%q,%q), want (%q,%q)", tc.in, gotProvider, gotName, tc.wantProvider, tc.wantName)
		}
	}
}

func TestTaskOutputBody(t *testing.T) {
	if got := taskOutputBody(map[string]any{"output": map[string]any{"raw_output": "hello world"}}); got != "hello world" {
		t.Errorf("raw_output: got %q", got)
	}
	if got := taskOutputBody(map[string]any{"output": map[string]any{"summary": "done"}}); got != "done" {
		t.Errorf("summary fallback: got %q", got)
	}
	if got := taskOutputBody(map[string]any{"output": map[string]any{"raw_output": "  ", "summary": "s"}}); got != "s" {
		t.Errorf("blank raw_output should fall back to summary: got %q", got)
	}
	if got := taskOutputBody(map[string]any{}); got != "" {
		t.Errorf("missing output: got %q", got)
	}
}

func TestRoomMessageForEvent(t *testing.T) {
	run := orch.Run{ID: "run-1", RoomID: "room-1"}
	taskByID := map[string]orch.Task{
		"t-ok":    {ID: "t-ok", Title: "实现后端", AssignedAgentID: "codex", Status: orch.TaskStatusSucceeded},
		"t-fail":  {ID: "t-fail", Title: "跑测试", AssignedAgentID: "claude-code", Status: orch.TaskStatusFailed},
		"t-retry": {ID: "t-retry", Title: "瞬时失败后成功", AssignedAgentID: "codex", Status: orch.TaskStatusSucceeded},
	}

	t.Run("run_planned surfaces task count", func(t *testing.T) {
		req, ok := roomMessageForEvent(orch.RunEvent{Type: orch.EventRunPlanned, Seq: 1, Payload: map[string]any{"tasks": float64(3)}}, run, taskByID)
		if !ok {
			t.Fatal("expected ok")
		}
		if req.SenderType != "system" || !strings.Contains(req.Body, "3 个子任务") {
			t.Errorf("unexpected planned msg: %+v", req)
		}
	})

	t.Run("task_succeeded carries output as agent message", func(t *testing.T) {
		req, ok := roomMessageForEvent(orch.RunEvent{
			Type: orch.EventTaskSucceeded, Seq: 5, TaskID: "t-ok",
			Payload: map[string]any{"output": map[string]any{"raw_output": "组件已生成"}},
		}, run, taskByID)
		if !ok {
			t.Fatal("expected ok")
		}
		if req.SenderType != "agent" || req.SenderName != "Codex" || req.Body != "组件已生成" {
			t.Errorf("unexpected succeeded msg: %+v", req)
		}
		if req.Metadata["task_id"] != "t-ok" || req.Metadata["event_seq"] != int64(5) {
			t.Errorf("metadata missing run/task linkage: %+v", req.Metadata)
		}
	})

	t.Run("terminal task_failed surfaces degraded notice", func(t *testing.T) {
		req, ok := roomMessageForEvent(orch.RunEvent{
			Type: orch.EventTaskFailed, Seq: 7, TaskID: "t-fail",
			Payload: map[string]any{"error": "boom"},
		}, run, taskByID)
		if !ok {
			t.Fatal("expected ok for terminal failure")
		}
		if !strings.Contains(req.Body, "失败") || !strings.Contains(req.Body, "boom") || req.Metadata["degraded"] != true {
			t.Errorf("unexpected failed msg: %+v", req)
		}
	})

	t.Run("retried-then-succeeded failure is skipped", func(t *testing.T) {
		// final task status is succeeded, so an intermediate failure event must not surface.
		if _, ok := roomMessageForEvent(orch.RunEvent{Type: orch.EventTaskFailed, Seq: 6, TaskID: "t-retry", Payload: map[string]any{"retryable": true}}, run, taskByID); ok {
			t.Error("expected transient failure to be skipped")
		}
	})

	t.Run("run completed surfaces summary", func(t *testing.T) {
		req, ok := roomMessageForEvent(orch.RunEvent{Type: orch.EventRunStatusChanged, Seq: 9, Payload: map[string]any{"to": string(orch.RunStatusCompleted)}}, run, taskByID)
		if !ok || !strings.Contains(req.Body, "完成") {
			t.Errorf("expected completion msg, got ok=%v req=%+v", ok, req)
		}
	})

	t.Run("non-terminal status change is skipped", func(t *testing.T) {
		if _, ok := roomMessageForEvent(orch.RunEvent{Type: orch.EventRunStatusChanged, Seq: 2, Payload: map[string]any{"to": string(orch.RunStatusDispatching)}}, run, taskByID); ok {
			t.Error("expected intermediate status change to be skipped")
		}
	})

	t.Run("unmapped event is skipped", func(t *testing.T) {
		if _, ok := roomMessageForEvent(orch.RunEvent{Type: orch.EventTaskCreated, Seq: 3, TaskID: "t-ok"}, run, taskByID); ok {
			t.Error("expected task_created to be skipped")
		}
	})
}

// stubAgentProvider is a canned orchestrator.AgentProvider for spine tests: it
// never touches a CLI/API and returns a fixed output per provider.
type stubAgentProvider struct {
	name string
	raw  string
}

func (s stubAgentProvider) Name() string         { return s.name }
func (stubAgentProvider) Capabilities() []string { return []string{"code", "review"} }
func (s stubAgentProvider) Execute(_ context.Context, _ orch.ExecuteTaskRequest) (orch.ExecuteTaskResult, error) {
	return orch.ExecuteTaskResult{Output: map[string]any{"raw_output": s.raw}, Summary: "done"}, nil
}

// twoAgentPlanner is a deterministic test planner that splits an objective into
// one parallel task per provider, so the projection test does not depend on the
// RulePlanner's keyword heuristics (which are covered in the orchestrator pkg).
type twoAgentPlanner struct{}

func (twoAgentPlanner) Plan(_ context.Context, _ orch.PlanInput) (orch.Plan, error) {
	return orch.Plan{
		PlannerVersion: "test/two-agent",
		Tasks: []orch.TaskDraft{
			{ClientKey: "backend", Title: "实现后端", Description: "实现后端接口", AssignedAgentID: "codex", ProviderName: "codex", Priority: 80, Timeout: time.Minute, MaxRetries: 1},
			{ClientKey: "frontend", Title: "实现前端", Description: "实现前端页面", AssignedAgentID: "claude-code", ProviderName: "claudecode", Priority: 80, Timeout: time.Minute, MaxRetries: 1},
		},
	}, nil
}

// TestRunEventsToMessages_RealEngine drives a real orchestrator engine (memory
// store + stub providers) through StartRun, then projects the real event stream
// into room messages. This exercises the full M1.1 spine: plan -> dispatch ->
// per-agent output -> completion, in order, with idempotent seq tracking.
func TestRunEventsToMessages_RealEngine(t *testing.T) {
	store := orch.NewMemoryStore()
	registry := orch.NewProviderRegistry(
		stubAgentProvider{name: "claudecode", raw: "Claude 的产出"},
		stubAgentProvider{name: "codex", raw: "Codex 的产出"},
		orch.NoopProvider{},
	)
	svc := orch.NewService(store, twoAgentPlanner{}, registry, nil, orch.Config{
		MaxParallelPerRun:   3,
		MaxParallelPerAgent: 1,
		DefaultTaskTimeout:  time.Minute,
		DispatchAsync:       false,
	})

	ctx := context.Background()
	snap, err := svc.StartRun(ctx, orch.StartRunInput{
		RoomID:    "room-1",
		Objective: "用 Claude Code 和 Codex 实现并验证一个功能",
		Agents: []orch.AgentDescriptor{
			{ID: "claude-code", ProviderName: "claudecode", Name: "Claude Code", Capabilities: []string{"frontend", "code"}},
			{ID: "codex", ProviderName: "codex", Name: "Codex", Capabilities: []string{"backend", "code"}},
		},
		AutoDispatch: true,
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if snap.Run.Status != orch.RunStatusCompleted {
		t.Fatalf("expected run completed, got %s", snap.Run.Status)
	}

	events, err := svc.ListEvents(ctx, snap.Run.ID, 0, 1000)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	msgs, maxSeq := runEventsToMessages(events, snap.Run, snap.Tasks, 0)
	if len(msgs) == 0 {
		t.Fatal("expected projected messages")
	}
	if maxSeq <= 0 {
		t.Errorf("expected positive maxSeq, got %d", maxSeq)
	}

	if msgs[0].SenderType != "system" || !strings.Contains(msgs[0].Body, "子任务") {
		t.Errorf("first message should be the plan summary, got %+v", msgs[0])
	}
	last := msgs[len(msgs)-1]
	if last.SenderType != "system" || !strings.Contains(last.Body, "完成") {
		t.Errorf("last message should be completion notice, got %+v", last)
	}

	var sawClaude, sawCodex bool
	for _, m := range msgs {
		if strings.TrimSpace(m.Body) == "" {
			t.Errorf("empty message body: %+v", m)
		}
		if m.SenderName == "Claude Code" && strings.Contains(m.Body, "Claude 的产出") {
			sawClaude = true
		}
		if m.SenderName == "Codex" && strings.Contains(m.Body, "Codex 的产出") {
			sawCodex = true
		}
	}
	if !sawClaude || !sawCodex {
		t.Errorf("expected both agent outputs surfaced: claude=%v codex=%v", sawClaude, sawCodex)
	}

	// Idempotency: ListEvents after the projected high-water seq returns nothing,
	// so a re-projection produces no duplicate messages.
	newEvents, err := svc.ListEvents(ctx, snap.Run.ID, maxSeq, 1000)
	if err != nil {
		t.Fatalf("ListEvents(afterSeq): %v", err)
	}
	if len(newEvents) != 0 {
		t.Errorf("expected no events after maxSeq=%d, got %d", maxSeq, len(newEvents))
	}
}
