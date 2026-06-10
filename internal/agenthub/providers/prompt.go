package providers

import (
	"strings"

	"github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

// PromptWithContext builds the CLI prompt for a task, prepending (a) the outputs
// of upstream tasks this task depends on (ExecuteTaskRequest.Upstream) and (b)
// the room's recent conversation history (Context["room_history"]) so the agent
// has the prior context it needs — e.g. "claude code 转述 qiling 的回答" can only
// work if qiling's produced answer is fed into claude code's prompt. The
// room_history snapshot is captured at run start, so a sibling agent's *new*
// reply during this same run reaches a dependent agent only via Upstream.
// Shared by the Claude Code, Codex, and Memoh providers so all behave identically.
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
	if up := upstreamOutputs(req.Upstream); up != "" {
		prompt = up + "\n\n" + prompt
	}
	return prompt
}

// upstreamOutputs renders the text outputs of successful upstream task attempts
// (those this task depends on) as a labelled block, so a dependent agent can see
// and build on what the upstream agents produced. Returns "" when there is no
// usable upstream output.
func upstreamOutputs(upstream []orchestrator.TaskAttempt) string {
	var b strings.Builder
	for _, att := range upstream {
		text := attemptOutputText(att.OutputPayload)
		if text == "" {
			continue
		}
		label := strings.TrimSpace(att.AgentID)
		if label == "" {
			label = strings.TrimSpace(att.ProviderName)
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		if label != "" {
			b.WriteString("[")
			b.WriteString(label)
			b.WriteString("]：\n")
		}
		b.WriteString(text)
	}
	if b.Len() == 0 {
		return ""
	}
	return "上游协作 Agent 的产出（你的任务需要基于/转述它们，请直接使用以下内容）：\n" + b.String()
}

// attemptOutputText extracts the agent's produced text from an attempt's output
// payload, preferring the full raw_output and falling back to the summary.
func attemptOutputText(output map[string]any) string {
	if output == nil {
		return ""
	}
	if raw, ok := output["raw_output"].(string); ok {
		if s := strings.TrimSpace(raw); s != "" {
			return s
		}
	}
	if sum, ok := output["summary"].(string); ok {
		return strings.TrimSpace(sum)
	}
	return ""
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
