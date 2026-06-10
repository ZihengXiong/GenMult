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

// WithWorkdirNote prepends a note telling the agent its working directory, which
// is shared by every CLI agent in the room — so it can read files other agents
// wrote and leave its output there for them ("挂载目录贡献给全群 agent，并在 prompt 注明").
func WithWorkdirNote(prompt, workDir string) string {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return prompt
	}
	return "你的工作目录是 " + workDir + "（本群所有 agent 共享同一目录：你可以读取其他 agent 在此写下的文件，也请把你的产出写到这里）。\n\n" + prompt
}
