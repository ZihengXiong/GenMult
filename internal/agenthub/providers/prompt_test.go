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
