package agenthub

import (
	"strings"
	"testing"

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
