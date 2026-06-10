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
