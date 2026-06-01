package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/agenthub/orchestrator"
)

// ClaudeEvent represents a raw JSON line output from the Claude CLI stream.
type ClaudeEvent struct {
	Type    string          `json:"type"`
	Role    string          `json:"role,omitempty"`
	Content json.RawMessage `json:"content,omitempty"`
	Message *ClaudeMessage  `json:"message,omitempty"`
}

// ClaudeMessage represents a message wrapper inside ClaudeEvent.
type ClaudeMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a single content piece in a Claude message.
type ContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Name  string `json:"name,omitempty"`
	ID    string `json:"id,omitempty"`
	Input any    `json:"input,omitempty"`
}

// ClaudeCodeProvider implements orchestrator.AgentProvider using Claude Code CLI.
type ClaudeCodeProvider struct {
	config ClaudeCodeConfig
	wsInfo WorkspaceResolver
	store  orchestrator.Store
	logger *slog.Logger
}

// NewClaudeCodeProvider creates a new ClaudeCodeProvider.
func NewClaudeCodeProvider(config ClaudeCodeConfig, wsInfo WorkspaceResolver, store orchestrator.Store, logger *slog.Logger) *ClaudeCodeProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &ClaudeCodeProvider{
		config: config,
		wsInfo: wsInfo,
		store:  store,
		logger: logger.With(slog.String("component", "claude_code_provider")),
	}
}

// Name returns the provider's registered name.
func (p *ClaudeCodeProvider) Name() string {
	return "claudecode"
}

// Capabilities returns the provider's supported capabilities.
func (p *ClaudeCodeProvider) Capabilities() []string {
	return []string{"code", "review"}
}

// Execute starts the Claude Code subprocess to fulfill a task.
func (p *ClaudeCodeProvider) Execute(ctx context.Context, req orchestrator.ExecuteTaskRequest) (orchestrator.ExecuteTaskResult, error) {
	workDir, err := p.wsInfo.ResolveWorkDir(ctx, req)
	if err != nil {
		return orchestrator.ExecuteTaskResult{Retryable: false}, fmt.Errorf("failed to resolve workspace directory: %w", err)
	}

	// Set up custom environment containing the API key.
	env := append(os.Environ(), "ANTHROPIC_API_KEY="+p.config.APIKey)
	if val := os.Getenv("ANTHROPIC_BASE_URL"); val != "" {
		env = append(env, "ANTHROPIC_BASE_URL="+val)
	}

	buildArgs := func(prompt string) []string {
		args := []string{
			"-p", prompt,
			"--output-format", "stream-json",
			"--verbose",
		}
		if p.config.PermissionMode != "" {
			args = append(args, "--permission-mode", p.config.PermissionMode)
		} else {
			args = append(args, "--permission-mode", "acceptEdits")
		}
		if p.config.MaxTurns > 0 {
			args = append(args, "--max-turns", strconv.Itoa(p.config.MaxTurns))
		} else {
			args = append(args, "--max-turns", "15")
		}
		if len(p.config.AllowedTools) > 0 {
			args = append(args, "--allowedTools", strings.Join(p.config.AllowedTools, ","))
		}
		if p.config.Model != "" {
			args = append(args, "--model", p.config.Model)
		}
		return args
	}

	parseEvent := func(line []byte) (CLIEvent, error) {
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
			_ = json.Unmarshal(ce.Content, &resultTxt)
			return CLIEvent{Type: "result", Content: resultTxt, Raw: line}, nil
		}
		return CLIEvent{Type: ce.Type, Raw: line}, nil
	}

	onEvent := func(event CLIEvent) {
		eventType := orchestrator.EventAgentOutput
		if event.Type == "tool_use" {
			eventType = orchestrator.EventAgentToolCall
		}

		// Fire-and-forget event forwarding.
		_, err := p.store.AppendEvent(ctx, orchestrator.RunEvent{
			ID:        uuid.NewString(),
			RunID:     req.Run.ID,
			TaskID:    req.Task.ID,
			Type:      eventType,
			Payload:   map[string]any{"content": event.Content, "raw_type": event.Type},
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			p.logger.Error("failed to append agent event to store", slog.String("error", err.Error()))
		}
	}

	runner := NewCLIRunner(CLIRunnerConfig{
		BinaryName: "claude",
		BuildArgs:  buildArgs,
		ParseEvent: parseEvent,
		OnEvent:    onEvent,
		Env:        env,
	}, p.logger)

	prompt := req.Task.Description
	if prompt == "" {
		prompt = req.Task.Title
	}

	output, err := runner.Run(ctx, prompt, workDir)
	if err != nil {
		if errors.Is(err, ErrCLINotFound) {
			return orchestrator.ExecuteTaskResult{Retryable: false}, err
		}
		errStr := err.Error()
		errLower := strings.ToLower(errStr)
		if strings.Contains(errLower, "rate limit") || strings.Contains(errLower, "429") {
			return orchestrator.ExecuteTaskResult{Retryable: true}, fmt.Errorf("%w: %s", ErrRateLimit, errStr)
		}
		if strings.Contains(errLower, "authentication") || strings.Contains(errLower, "401") || strings.Contains(errLower, "unauthorized") {
			return orchestrator.ExecuteTaskResult{Retryable: false}, fmt.Errorf("%w: %s", ErrAuthFailure, errStr)
		}
		return orchestrator.ExecuteTaskResult{Retryable: true}, err
	}

	return orchestrator.ExecuteTaskResult{
		Output:    map[string]any{"raw_output": output},
		Summary:   "Claude Code execution completed successfully",
		Retryable: false,
	}, nil
}
