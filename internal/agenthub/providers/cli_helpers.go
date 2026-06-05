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
	// Third-party Anthropic-compatible APIs (e.g. DeepSeek) use Bearer auth
	// (ANTHROPIC_AUTH_TOKEN) rather than the Anthropic-specific x-api-key header
	// (ANTHROPIC_API_KEY). Check both the explicit config and the environment
	// variable, since the base URL may be injected via ANTHROPIC_BASE_URL without
	// being set in the bot's claudecode config.
	effectiveBaseURL := cfg.BaseURL
	if effectiveBaseURL == "" {
		effectiveBaseURL = os.Getenv("ANTHROPIC_BASE_URL")
	}
	thirdParty := effectiveBaseURL != "" && !strings.Contains(effectiveBaseURL, "api.anthropic.com")
	if cfg.AuthToken != "" {
		env = append(env, "ANTHROPIC_AUTH_TOKEN="+cfg.AuthToken)
		if thirdParty {
			env = append(env, "ANTHROPIC_API_KEY=") // suppress x-api-key for third-party endpoints
		}
	} else if cfg.APIKey != "" {
		if thirdParty {
			env = append(env, "ANTHROPIC_AUTH_TOKEN="+cfg.APIKey)
			env = append(env, "ANTHROPIC_API_KEY=") // suppress x-api-key for third-party endpoints
		} else {
			env = append(env, "ANTHROPIC_API_KEY="+cfg.APIKey)
		}
	}
	if cfg.BaseURL != "" {
		env = append(env, "ANTHROPIC_BASE_URL="+cfg.BaseURL)
	} else if val := os.Getenv("ANTHROPIC_BASE_URL"); val != "" {
		env = append(env, "ANTHROPIC_BASE_URL="+val)
	}
	for k, v := range cfg.CustomEnv {
		if k != "" && v != "" {
			env = append(env, k+"="+v)
		}
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
		// Only emit init for the "init" subtype; skip noisy subtypes like
		// "thinking_tokens" which are just token counters.
		if ce.Subtype == "init" {
			return CLIEvent{Type: "init", Content: "initialized", SessionID: ce.SessionID, Raw: line}, nil
		}
		// Silently skip other system subtypes (thinking_tokens, etc.)
		return CLIEvent{Type: "system", Raw: line}, nil
	case "assistant":
		if ce.Message != nil {
			var thinkings []string
			var texts []string
			var tools []string
			var firstInput any
			for _, block := range ce.Message.Content {
				switch block.Type {
				case "thinking":
					if block.Thinking != "" {
						thinkings = append(thinkings, block.Thinking)
					}
				case "text":
					texts = append(texts, block.Text)
				case "tool_use":
					tools = append(tools, block.Name)
					if firstInput == nil {
						firstInput = block.Input
					}
				}
			}
			// Prefer thinking content if present (Claude's internal reasoning).
			// Fall back to text content (Claude's visible reply).
			if len(thinkings) > 0 || len(texts) > 0 || len(tools) > 0 {
				ev := CLIEvent{Raw: line}
				switch {
				case len(thinkings) > 0:
					ev.Type = "thinking"
					ev.Content = strings.Join(thinkings, "")
					// Preserve visible text alongside thinking so the caller can emit both.
					if len(texts) > 0 {
						ev.TextContent = strings.Join(texts, "")
					}
				case len(texts) > 0:
					ev.Type = "text"
					ev.Content = strings.Join(texts, "")
				default:
					ev.Type = "tool_use"
					ev.Content = strings.Join(tools, ", ")
				}
				if len(tools) > 0 {
					ev.ToolName = strings.Join(tools, ", ")
					ev.Payload = firstInput
				}
				return ev, nil
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
