package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

// ClaudeEvent represents a raw JSON line output from the Claude CLI stream.
type ClaudeEvent struct {
	Type        string          `json:"type"`
	Subtype     string          `json:"subtype,omitempty"`
	Role        string          `json:"role,omitempty"`
	Content     json.RawMessage `json:"content,omitempty"`
	Message     *ClaudeMessage  `json:"message,omitempty"`
	Result      string          `json:"result,omitempty"`
	IsError     bool            `json:"is_error,omitempty"`
	ErrorStatus int             `json:"api_error_status,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
}

// ClaudeMessage represents a message wrapper inside ClaudeEvent.
type ClaudeMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a single content piece in a Claude message.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Name      string          `json:"name,omitempty"`
	ID        string          `json:"id,omitempty"`
	Input     any             `json:"input,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
}

// ClaudeCodeProvider implements orchestrator.AgentProvider using Claude Code CLI.
type ClaudeCodeProvider struct {
	config   ClaudeCodeConfig
	wsInfo   WorkspaceResolver
	store    orchestrator.Store
	executor CommandExecutor
	// resolveCreds optionally resolves {apiKey, baseURL} at execute time from the
	// configured provider store (the "通用设置" DeepSeek/Anthropic provider), so the
	// orchestrator reuses the same credential source as single-chat instead of
	// requiring ANTHROPIC_API_KEY in the environment. May be nil. Mirrors
	// botruntime/cli.go's resolveCreds callback.
	resolveCreds func(ctx context.Context) (apiKey, baseURL string)
	logger       *slog.Logger
}

// NewClaudeCodeProvider creates a new ClaudeCodeProvider. resolveCreds may be nil
// (then only the static config / env credentials are used).
func NewClaudeCodeProvider(config ClaudeCodeConfig, wsInfo WorkspaceResolver, store orchestrator.Store, executor CommandExecutor, resolveCreds func(ctx context.Context) (apiKey, baseURL string), logger *slog.Logger) *ClaudeCodeProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &ClaudeCodeProvider{
		config:       config,
		wsInfo:       wsInfo,
		store:        store,
		executor:     executor,
		resolveCreds: resolveCreds,
		logger:       logger.With(slog.String("component", "claude_code_provider")),
	}
}

// Name returns the provider's registered name.
func (*ClaudeCodeProvider) Name() string {
	return "claudecode"
}

// Capabilities returns the provider's supported capabilities.
func (*ClaudeCodeProvider) Capabilities() []string {
	return []string{"code", "review"}
}

// Execute starts the Claude Code subprocess to fulfill a task.
func (p *ClaudeCodeProvider) Execute(ctx context.Context, req orchestrator.ExecuteTaskRequest) (orchestrator.ExecuteTaskResult, error) {
	// Resolve credentials the same way single-chat does: prefer the static config,
	// then fall back to the configured provider store (DeepSeek/Anthropic-compatible
	// base_url + key set in 通用设置). Only fill what the local config left empty.
	cfg := p.config
	if p.resolveCreds != nil {
		if apiKey, baseURL := p.resolveCreds(ctx); apiKey != "" || baseURL != "" {
			if apiKey != "" && cfg.APIKey == "" && cfg.AuthToken == "" {
				cfg.APIKey = apiKey
			}
			if baseURL != "" && cfg.BaseURL == "" {
				cfg.BaseURL = baseURL
			}
		}
	}
	// A Bearer AuthToken (third-party) is as valid as an x-api-key, so accept either.
	if cfg.APIKey == "" && cfg.AuthToken == "" {
		return orchestrator.ExecuteTaskResult{Retryable: false}, fmt.Errorf("%w: 请在通用设置中配置 Anthropic/DeepSeek 兼容 provider（base_url + token），或设置 ANTHROPIC_API_KEY / ANTHROPIC_AUTH_TOKEN", ErrAPIKeyMissing)
	}

	workDir, err := p.wsInfo.ResolveWorkDir(ctx, req)
	if err != nil {
		return orchestrator.ExecuteTaskResult{Retryable: false}, fmt.Errorf("failed to resolve workspace directory: %w", err)
	}

	p.logger.Info("starting Claude Code task execution",
		slog.String("task_id", req.Task.ID),
		slog.String("run_id", req.Run.ID),
		slog.String("work_dir", workDir),
	)

	// Set up custom environment containing the resolved credentials.
	env := ClaudeEnv(cfg)

	buildArgs := func(prompt string) []string {
		return ClaudeBuildArgs(cfg, prompt)
	}

	parseEvent := ClaudeParseEvent

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
	}, p.logger)

	prompt := WithWorkdirNote(PromptWithContext(req), workDir)

	output, err := runner.Run(ctx, prompt, workDir, p.executor, env)
	if err != nil {
		p.logger.Error("Claude Code execution failed",
			slog.String("task_id", req.Task.ID),
			slog.String("run_id", req.Run.ID),
			slog.Any("error", err),
		)
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

	p.logger.Info("Claude Code task execution completed successfully",
		slog.String("task_id", req.Task.ID),
		slog.String("run_id", req.Run.ID),
	)

	return orchestrator.ExecuteTaskResult{
		Output:    map[string]any{"raw_output": output},
		Summary:   "Claude Code execution completed successfully",
		Retryable: false,
	}, nil
}
