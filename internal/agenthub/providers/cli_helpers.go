package providers

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// This file hosts request-free helpers shared by the orchestrator AgentProvider
// implementations (claudecode.go / codex.go) and other callers that drive the
// same CLIs outside the task-orchestration context (e.g. bot runtimes). Keeping
// argument construction, NDJSON parsing, and environment setup here avoids
// duplicating the provider-specific contracts.

// ClaudeEnv returns the environment for the Claude Code CLI: the current
// process environment with the configured API key and optional base URL
// set (replacing any existing values).
func ClaudeEnv(cfg ClaudeCodeConfig) []string {
	var env []string
	if cfg.APIKey != "" {
		env = append(env, "ANTHROPIC_API_KEY="+cfg.APIKey)
	}
	if cfg.BaseURL != "" {
		env = append(env, "ANTHROPIC_BASE_URL="+cfg.BaseURL)
	} else if val := os.Getenv("ANTHROPIC_BASE_URL"); val != "" {
		env = append(env, "ANTHROPIC_BASE_URL="+val)
	}
	return env
}

// ClaudeBuildArgs builds the Claude Code CLI arguments for a prompt.
func ClaudeBuildArgs(cfg ClaudeCodeConfig, prompt string) []string {
	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
	}
	if cfg.PermissionMode != "" {
		args = append(args, "--permission-mode", cfg.PermissionMode)
	} else {
		args = append(args, "--permission-mode", "auto")
	}
	if cfg.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(cfg.MaxTurns))
	} else {
		args = append(args, "--max-turns", "15")
	}
	if len(cfg.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(cfg.AllowedTools, ","))
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	return args
}

// ClaudeParseEvent parses one NDJSON line emitted by the Claude Code CLI into a
// provider-agnostic CLIEvent.
func ClaudeParseEvent(line []byte) (CLIEvent, error) {
	var ce ClaudeEvent
	if err := json.Unmarshal(line, &ce); err != nil {
		return CLIEvent{}, err
	}
	switch ce.Type {
	case "system":
		return CLIEvent{Type: "init", Content: "initialized", Raw: line}, nil
	case "assistant":
		if ce.Message != nil {
			var texts []string
			var tools []string
			for _, block := range ce.Message.Content {
				if block.Type == "text" {
					texts = append(texts, block.Text)
				} else if block.Type == "tool_use" {
					tools = append(tools, block.Name)
				}
			}
			if len(tools) > 0 {
				return CLIEvent{Type: "tool_use", Content: strings.Join(tools, ", "), Raw: line}, nil
			}
			if len(texts) > 0 {
				return CLIEvent{Type: "text", Content: strings.Join(texts, ""), Raw: line}, nil
			}
		}
		if ce.Content != nil {
			var txt string
			if err := json.Unmarshal(ce.Content, &txt); err == nil && txt != "" {
				return CLIEvent{Type: "text", Content: txt, Raw: line}, nil
			}
		}
	case "tool_result":
		var resultTxt string
		_ = json.Unmarshal(ce.Content, &resultTxt)
		return CLIEvent{Type: "tool_result", Content: resultTxt, Raw: line}, nil
	case "result":
		var resultTxt string
		if len(ce.Content) > 0 {
			_ = json.Unmarshal(ce.Content, &resultTxt)
		}
		if resultTxt == "" && ce.Result != "" {
			resultTxt = ce.Result
		}
		if ce.IsError {
			return CLIEvent{Type: "error", Content: resultTxt, Raw: line}, nil
		}
		return CLIEvent{Type: "result", Content: resultTxt, Raw: line}, nil
	}
	return CLIEvent{Type: ce.Type, Raw: line}, nil
}

// CodexEnv returns the environment for the Codex CLI: the current process
// environment with the configured API key and optional base URL
// set (replacing any existing values).
func CodexEnv(cfg CodexConfig) []string {
	var env []string
	if cfg.APIKey != "" {
		env = append(env, "OPENAI_API_KEY="+cfg.APIKey)
	}
	if cfg.BaseURL != "" {
		env = append(env, "OPENAI_BASE_URL="+cfg.BaseURL)
	} else if val := os.Getenv("OPENAI_BASE_URL"); val != "" {
		env = append(env, "OPENAI_BASE_URL="+val)
	}
	return env
}



// CodexBuildArgs builds the Codex CLI arguments for a prompt.
func CodexBuildArgs(cfg CodexConfig, prompt string) []string {
	args := []string{
		"exec",
		"--json",
		"--skip-git-repo-check",
	}
	if cfg.Sandbox != "" {
		args = append(args, "--sandbox", cfg.Sandbox)
	} else {
		args = append(args, "--sandbox", "workspace-write")
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	args = append(args, prompt)
	return args
}

// CodexParseEvent parses one NDJSON line emitted by the Codex CLI into a
// provider-agnostic CLIEvent.
func CodexParseEvent(line []byte) (CLIEvent, error) {
	var ce CodexEvent
	if err := json.Unmarshal(line, &ce); err != nil {
		return CLIEvent{}, err
	}
	switch ce.Type {
	case "thread.started":
		return CLIEvent{Type: "init", Content: "thread started", Raw: line}, nil
	case "turn.started":
		return CLIEvent{Type: "turn", Content: "turn started", Raw: line}, nil
	case "item.completed":
		if ce.Item != nil {
			if ce.Item.Type == "message" {
				return CLIEvent{Type: "text", Content: ce.Item.Content, Raw: line}, nil
			}
			if ce.Item.Type == "command" {
				return CLIEvent{Type: "tool_use", Content: ce.Item.Name, Raw: line}, nil
			}
		}
	case "turn.completed":
		content := ce.Summary
		if content == "" && ce.Item != nil {
			content = ce.Item.Content
		}
		return CLIEvent{Type: "result", Content: content, Raw: line}, nil
	case "error":
		content := "unknown codex error"
		if ce.Error != nil {
			content = *ce.Error
		}
		return CLIEvent{Type: "error", Content: content, Raw: line}, nil
	}
	return CLIEvent{Type: ce.Type, Raw: line}, nil
}
