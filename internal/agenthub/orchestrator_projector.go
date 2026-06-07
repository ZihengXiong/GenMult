package agenthub

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	orch "github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

// projectRun turns new orchestrator run events into room chat messages so that
// a run's planning, per-agent outputs, and final result surface in the AgentHub
// room timeline ("在聊天流中汇报结果"). It is idempotent: each run's last
// projected event seq is tracked in-memory and ListEvents(afterSeq) only returns
// newer events.
//
// Granularity is per-task-completion (one message per task result), matching the
// M1 design; token-level streaming is a future enhancement. Projection is driven
// synchronously from the wiring StartRun/ReconcileRun calls (DispatchAsync is off).
func (s *OrchestratorService) projectRun(ctx context.Context, ownerUserID string, snap orch.RunSnapshot) {
	if s == nil || s.rooms == nil || s.orch == nil {
		return
	}
	runID := strings.TrimSpace(snap.Run.ID)
	roomID := strings.TrimSpace(snap.Run.RoomID)
	if runID == "" || roomID == "" {
		return
	}

	s.projMu.Lock()
	after := s.projSeq[runID]
	s.projMu.Unlock()

	events, err := s.orch.ListEvents(ctx, runID, after, 1000)
	if err != nil {
		s.log.Error("project run: list events failed", slog.String("run_id", runID), slog.Any("error", err))
		return
	}
	if len(events) == 0 {
		return
	}

	taskByID := make(map[string]orch.Task, len(snap.Tasks))
	for _, task := range snap.Tasks {
		taskByID[task.ID] = task
	}

	maxSeq := after
	for _, ev := range events {
		if ev.Seq > maxSeq {
			maxSeq = ev.Seq
		}
		req, ok := roomMessageForEvent(ev, snap.Run, taskByID)
		if !ok {
			continue
		}
		if _, err := s.rooms.CreateMessage(ctx, ownerUserID, roomID, req); err != nil {
			s.log.Error("project run: create room message failed",
				slog.String("run_id", runID),
				slog.Int64("seq", ev.Seq),
				slog.String("event_type", string(ev.Type)),
				slog.Any("error", err),
			)
		}
	}

	s.projMu.Lock()
	if maxSeq > s.projSeq[runID] {
		s.projSeq[runID] = maxSeq
	}
	s.projMu.Unlock()
}

// roomMessageForEvent maps a single orchestrator RunEvent to a room message.
// The bool is false for events that should not surface in the chat stream.
func roomMessageForEvent(ev orch.RunEvent, run orch.Run, taskByID map[string]orch.Task) (CreateMessageRequest, bool) {
	meta := func(extra map[string]any) map[string]any {
		m := map[string]any{
			"run_id":     run.ID,
			"event_seq":  ev.Seq,
			"event_type": string(ev.Type),
		}
		if ev.TaskID != "" {
			m["task_id"] = ev.TaskID
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	switch ev.Type {
	case orch.EventRunPlanned:
		n := intFromPayload(ev.Payload, "tasks")
		return CreateMessageRequest{
			SenderID:   "orchestrator",
			SenderType: "system",
			SenderName: "Orchestrator",
			Kind:       "orchestrator",
			Title:      "任务规划",
			Body:       "📋 已理解目标并拆解为 " + strconv.Itoa(n) + " 个子任务，开始分派执行。",
			Metadata:   meta(nil),
		}, true

	case orch.EventTaskDispatched:
		task, ok := taskByID[ev.TaskID]
		if !ok {
			return CreateMessageRequest{}, false
		}
		_, name := friendlyAgentName(task.AssignedAgentID)
		return CreateMessageRequest{
			SenderID:   task.AssignedAgentID,
			SenderType: "agent",
			SenderName: name,
			Kind:       "status",
			Title:      task.Title,
			Body:       "▶️ 开始执行：" + task.Title,
			Metadata:   meta(nil),
		}, true

	case orch.EventTaskSucceeded:
		task := taskByID[ev.TaskID]
		_, name := friendlyAgentName(task.AssignedAgentID)
		body := taskOutputBody(ev.Payload)
		if strings.TrimSpace(body) == "" {
			body = "✅ 已完成：" + task.Title
		}
		title := task.Title
		if title == "" {
			title = name
		}
		return CreateMessageRequest{
			SenderID:   task.AssignedAgentID,
			SenderType: "agent",
			SenderName: name,
			Kind:       "message",
			Title:      title,
			Body:       body,
			Metadata:   meta(nil),
		}, true

	case orch.EventTaskFailed, orch.EventTaskTimedOut:
		task, ok := taskByID[ev.TaskID]
		if !ok {
			return CreateMessageRequest{}, false
		}
		// Only surface terminal failures. A transient failure that was retried
		// and later succeeded ends with a non-failed status, so skip it to avoid
		// posting a "failed" message followed by a "succeeded" one.
		if task.Status != orch.TaskStatusFailed && task.Status != orch.TaskStatusBlocked && task.Status != orch.TaskStatusCancelled {
			return CreateMessageRequest{}, false
		}
		_, name := friendlyAgentName(task.AssignedAgentID)
		prefix := "⚠️ 任务失败"
		if ev.Type == orch.EventTaskTimedOut {
			prefix = "⏱️ 任务超时"
		}
		body := prefix + "：" + task.Title
		if errMsg, ok := ev.Payload["error"].(string); ok && strings.TrimSpace(errMsg) != "" {
			body += "\n原因：" + errMsg
		}
		body += "\n（已降级，继续执行其余任务）"
		return CreateMessageRequest{
			SenderID:   task.AssignedAgentID,
			SenderType: "agent",
			SenderName: name,
			Kind:       "status",
			Title:      task.Title,
			Body:       body,
			Metadata:   meta(map[string]any{"degraded": true}),
		}, true

	case orch.EventRunStatusChanged:
		to, _ := ev.Payload["to"].(string)
		switch orch.RunStatus(to) {
		case orch.RunStatusCompleted:
			return CreateMessageRequest{
				SenderID:   "orchestrator",
				SenderType: "system",
				SenderName: "Orchestrator",
				Kind:       "orchestrator",
				Title:      "协作完成",
				Body:       "✅ 本次多 Agent 协作已完成。",
				Metadata:   meta(nil),
			}, true
		case orch.RunStatusFailed:
			return CreateMessageRequest{
				SenderID:   "orchestrator",
				SenderType: "system",
				SenderName: "Orchestrator",
				Kind:       "orchestrator",
				Title:      "协作结束",
				Body:       "❌ 本次协作以失败结束（部分任务未完成，详见上方各任务说明）。",
				Metadata:   meta(nil),
			}, true
		default:
			return CreateMessageRequest{}, false
		}

	default:
		return CreateMessageRequest{}, false
	}
}

// taskOutputBody extracts human-readable text from a task_succeeded event payload.
// Providers store their full text in output.raw_output and a short summary in
// output.summary (see providers.ClaudeCodeProvider.Execute).
func taskOutputBody(payload map[string]any) string {
	out, ok := payload["output"].(map[string]any)
	if !ok {
		return ""
	}
	if raw, ok := out["raw_output"].(string); ok && strings.TrimSpace(raw) != "" {
		return raw
	}
	if summary, ok := out["summary"].(string); ok {
		return summary
	}
	return ""
}

func intFromPayload(payload map[string]any, key string) int {
	switch v := payload[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// friendlyAgentName maps an agent id to its orchestrator provider name and a
// human-friendly display name. Shared by agentsFromRoom and the room projector.
func friendlyAgentName(agentID string) (provider string, name string) {
	id := strings.TrimSpace(agentID)
	lc := strings.ToLower(id)
	switch {
	case strings.Contains(lc, "claude"):
		return "claudecode", "Claude Code"
	case strings.Contains(lc, "codex"):
		return "codex", "Codex"
	case id == "" || strings.Contains(lc, "orchestrator"):
		return "noop", "Orchestrator"
	default:
		return "noop", id
	}
}
