package providers

import (
	"strings"

	"github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

// PromptWithContext builds the CLI prompt for a task, prepending the room's
// recent conversation history (when the orchestrator supplied it via
// ExecuteTaskRequest.Context["room_history"]) so the agent has multi-turn
// context ("上下文连续 / 多轮迭代修改"). Shared by the Claude Code and Codex
// providers so both behave identically.
func PromptWithContext(req orchestrator.ExecuteTaskRequest) string {
	prompt := strings.TrimSpace(req.Task.Description)
	if prompt == "" {
		prompt = strings.TrimSpace(req.Task.Title)
	}
	if hist, ok := req.Context["room_history"].(string); ok {
		if h := strings.TrimSpace(hist); h != "" {
			prompt = "群聊对话历史（仅供理解上下文，请聚焦“当前任务”）：\n" + h + "\n\n当前任务：\n" + prompt
		}
	}
	return prompt
}
