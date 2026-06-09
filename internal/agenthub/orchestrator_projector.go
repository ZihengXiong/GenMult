package agenthub

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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

	msgs, maxSeq := runEventsToMessages(events, snap.Run, snap.Tasks, after)
	for i, req := range msgs {
		if _, err := s.rooms.CreateMessage(ctx, ownerUserID, roomID, req); err != nil {
			s.log.Error("project run: create room message failed",
				slog.String("run_id", runID),
				slog.Int("message_index", i),
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

// runEventsToMessages maps an ordered slice of orchestrator run events to the
// room messages that should be posted, returning them in order along with the
// highest event seq seen (for idempotent projection). It is pure (no IO) so it
// can be tested against a real engine's event stream.
func runEventsToMessages(events []orch.RunEvent, run orch.Run, tasks []orch.Task, after int64) ([]CreateMessageRequest, int64) {
	taskByID := make(map[string]orch.Task, len(tasks))
	for _, task := range tasks {
		taskByID[task.ID] = task
	}
	out := make([]CreateMessageRequest, 0, len(events))
	maxSeq := after
	surfacedAgentOutput := make(map[string]map[string]bool)
	for _, ev := range events {
		if ev.Seq > maxSeq {
			maxSeq = ev.Seq
		}
		if ev.Type == orch.EventTaskSucceeded {
			body := normalizeProjectedOutput(taskOutputBody(ev.Payload))
			if body != "" && surfacedAgentOutput[ev.TaskID][body] {
				continue
			}
		}
		req, ok := roomMessageForEvent(ev, run, taskByID)
		if !ok {
			continue
		}
		out = append(out, req)
		if ev.Type == orch.EventAgentOutput {
			body := normalizeProjectedOutput(req.Body)
			if body != "" {
				if surfacedAgentOutput[ev.TaskID] == nil {
					surfacedAgentOutput[ev.TaskID] = make(map[string]bool)
				}
				surfacedAgentOutput[ev.TaskID][body] = true
			}
		}
	}
	return out, maxSeq
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
		body := plannedRunBody(ev.Payload, taskByID)
		return CreateMessageRequest{
			SenderID:   "orchestrator",
			SenderType: "system",
			SenderName: "Orchestrator",
			Kind:       "orchestrator",
			Title:      "任务规划",
			Body:       body,
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

	case orch.EventAgentOutput:
		task, ok := taskByID[ev.TaskID]
		if !ok {
			return CreateMessageRequest{}, false
		}
		body := agentOutputBody(ev.Payload)
		if strings.TrimSpace(body) == "" {
			return CreateMessageRequest{}, false
		}
		_, name := friendlyAgentName(task.AssignedAgentID)
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
			Metadata:   meta(map[string]any{"streamed": true}),
		}, true

	case orch.EventTaskSucceeded:
		task, ok := taskByID[ev.TaskID]
		if !ok {
			return CreateMessageRequest{}, false
		}
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
		switch orch.RunStatus(payloadString(ev.Payload, "to")) {
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

func plannedRunBody(payload map[string]any, taskByID map[string]orch.Task) string {
	n := intFromPayload(payload, "tasks")
	base := "📋 已理解目标并拆解为 " + strconv.Itoa(n) + " 个子任务，开始分派执行。"
	if len(taskByID) == 0 {
		return base
	}
	tasks := make([]orch.Task, 0, len(taskByID))
	for _, task := range taskByID {
		tasks = append(tasks, task)
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		}
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority > tasks[j].Priority
		}
		return tasks[i].ID < tasks[j].ID
	})
	lines := []string{base, "", "任务清单："}
	for i, task := range tasks {
		_, name := friendlyAgentName(task.AssignedAgentID)
		title := strings.TrimSpace(task.Title)
		if title == "" {
			title = "子任务"
		}
		if name == "" || name == "Orchestrator" {
			name = strings.TrimSpace(task.ProviderName)
		}
		if name == "" {
			name = "Agent"
		}
		lines = append(lines, fmt.Sprintf("%d. %s：%s", i+1, name, title))
	}
	return strings.Join(lines, "\n")
}

// agentOutputBody extracts visible provider output from an agent_output event.
// It intentionally skips init/turn/tool noise and Claude thinking blocks: the
// room timeline should show user-visible agent text, not process logs.
func agentOutputBody(payload map[string]any) string {
	content, _ := payload["content"].(string)
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	rawType, _ := payload["raw_type"].(string)
	switch strings.ToLower(strings.TrimSpace(rawType)) {
	case "", "text", "result", "error":
		return content
	default:
		return ""
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

func normalizeProjectedOutput(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// payloadString coerces an event payload value to a string, tolerating both the
// typed values held by the in-memory store (e.g. orch.RunStatus) and the plain
// strings produced after a JSON round-trip through the SQL store.
func payloadString(payload map[string]any, key string) string {
	switch v := payload[key].(type) {
	case nil:
		return ""
	case string:
		return v
	case orch.RunStatus:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
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
