package providers

import (
	"strings"
	"testing"

	"github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

func TestPromptWithContext(t *testing.T) {
	base := orchestrator.ExecuteTaskRequest{Task: orchestrator.Task{Description: "实现登录"}}
	if got := PromptWithContext(base); got != "实现登录" {
		t.Errorf("no-context: got %q", got)
	}

	withHist := orchestrator.ExecuteTaskRequest{
		Task:    orchestrator.Task{Description: "实现登录"},
		Context: map[string]any{"room_history": "我：做个登录\nClaude Code：好的"},
	}
	got := PromptWithContext(withHist)
	if !strings.Contains(got, "做个登录") || !strings.Contains(got, "当前任务") || !strings.Contains(got, "实现登录") {
		t.Errorf("with-context prompt missing parts: %q", got)
	}

	titleOnly := orchestrator.ExecuteTaskRequest{Task: orchestrator.Task{Title: "标题任务"}}
	if got := PromptWithContext(titleOnly); got != "标题任务" {
		t.Errorf("title fallback: got %q", got)
	}

	blankHist := orchestrator.ExecuteTaskRequest{
		Task:    orchestrator.Task{Description: "x"},
		Context: map[string]any{"room_history": "   "},
	}
	if got := PromptWithContext(blankHist); got != "x" {
		t.Errorf("blank history should be ignored: got %q", got)
	}
}

func TestPromptWithContext_InjectsUpstreamOutput(t *testing.T) {
	// A dependent task (转述) must receive the upstream agent's produced answer,
	// since the room_history snapshot predates the upstream's reply in this run.
	req := orchestrator.ExecuteTaskRequest{
		Task: orchestrator.Task{Description: "转述 qiling 的自我介绍"},
		Upstream: []orchestrator.TaskAttempt{
			{
				AgentID:       "bot:qiling",
				OutputPayload: map[string]any{"raw_output": "我是 Astra ✦，你的数字搭档"},
			},
			// no usable output → skipped
			{AgentID: "bot:empty", OutputPayload: map[string]any{"raw_output": "  "}},
		},
	}
	got := PromptWithContext(req)
	if !strings.Contains(got, "我是 Astra ✦") || !strings.Contains(got, "bot:qiling") {
		t.Errorf("upstream output not injected: %q", got)
	}
	if !strings.Contains(got, "转述 qiling 的自我介绍") {
		t.Errorf("current task missing: %q", got)
	}
	if strings.Contains(got, "bot:empty") {
		t.Errorf("empty upstream output should be skipped: %q", got)
	}

	// summary fallback when raw_output absent
	sumReq := orchestrator.ExecuteTaskRequest{
		Task:     orchestrator.Task{Description: "do"},
		Upstream: []orchestrator.TaskAttempt{{ProviderName: "memoh", OutputPayload: map[string]any{"summary": "做完了登录"}}},
	}
	if got := PromptWithContext(sumReq); !strings.Contains(got, "做完了登录") {
		t.Errorf("summary fallback not used: %q", got)
	}
}

func TestClampMiddle(t *testing.T) {
	if got := ClampMiddle("short", 100); got != "short" {
		t.Errorf("within budget should be unchanged: %q", got)
	}
	long := strings.Repeat("头", 500) + strings.Repeat("尾", 500)
	got := ClampMiddle(long, 200)
	if runes := len([]rune(got)); runes > 200 {
		t.Errorf("clamped length = %d runes, want <= 200", runes)
	}
	if !strings.HasPrefix(got, "头") || !strings.HasSuffix(got, "尾") {
		t.Errorf("clamp should keep head and tail: %q", got)
	}
	if !strings.Contains(got, "已截断") {
		t.Errorf("clamp should mark the elision: %q", got)
	}
	if got := ClampMiddle(long, 0); got != long {
		t.Errorf("non-positive max should be a no-op")
	}
}

// TestUpstreamOutputsBounded guards the prompt budget: a huge upstream output
// must be clamped (keeping its head and tail) instead of inflating the
// dependent task's CLI prompt without bound.
func TestUpstreamOutputsBounded(t *testing.T) {
	huge := "BEGIN-MARKER " + strings.Repeat("x", UpstreamAttemptMaxChars*3) + " END-MARKER"
	req := orchestrator.ExecuteTaskRequest{
		Task: orchestrator.Task{Description: "转述上游产出"},
		Upstream: []orchestrator.TaskAttempt{{
			AgentID:       "qiling",
			Status:        orchestrator.AttemptStatusSucceeded,
			OutputPayload: map[string]any{"raw_output": huge},
		}},
	}
	got := PromptWithContext(req)
	if runes := len([]rune(got)); runes > UpstreamTotalMaxChars+1000 {
		t.Errorf("prompt length = %d runes, want bounded by upstream budget", runes)
	}
	for _, want := range []string{"BEGIN-MARKER", "END-MARKER", "已截断", "转述上游产出", "qiling"} {
		if !strings.Contains(got, want) {
			t.Errorf("bounded prompt missing %q", want)
		}
	}
}

// TestUpstreamOutputsUseDisplayNames: the upstream block must label outputs
// with the agent's display name from run metadata (surviving the JSON
// map[string]any round-trip) instead of the opaque bot UUID, and fall back to
// the raw id when no name is known.
func TestUpstreamOutputsUseDisplayNames(t *testing.T) {
	req := orchestrator.ExecuteTaskRequest{
		Task: orchestrator.Task{Description: "汇总"},
		Context: map[string]any{
			// JSON round-trip shape: map[string]any with string values.
			"agent_names": map[string]any{"bot:1111": "前端"},
		},
		Upstream: []orchestrator.TaskAttempt{
			{AgentID: "bot:1111", Status: orchestrator.AttemptStatusSucceeded, OutputPayload: map[string]any{"raw_output": "页面已完成"}},
			{AgentID: "bot:2222", Status: orchestrator.AttemptStatusSucceeded, OutputPayload: map[string]any{"raw_output": "接口已完成"}},
		},
	}
	got := PromptWithContext(req)
	if !strings.Contains(got, "[前端]：") {
		t.Fatalf("expected display-name label, got %q", got)
	}
	if strings.Contains(got, "[bot:1111]") {
		t.Fatalf("named agent must not be labelled by raw id, got %q", got)
	}
	if !strings.Contains(got, "[bot:2222]") {
		t.Fatalf("unnamed agent must fall back to raw id, got %q", got)
	}

	// In-process shape (no SQL round-trip yet): map[string]string.
	req.Context = map[string]any{"agent_names": map[string]string{"bot:1111": "前端"}}
	if got := PromptWithContext(req); !strings.Contains(got, "[前端]：") {
		t.Fatalf("map[string]string shape must work too, got %q", got)
	}
}
