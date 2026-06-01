package providers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/agenthub/orchestrator"
)

// CodexEvent represents a raw JSON line output from the Codex CLI stream.
type CodexEvent struct {
	Type  string     `json:"type"`
	Item  *CodexItem `json:"item,omitempty"`
	Error *string    `json:"error,omitempty"`
}

// CodexItem represents an item completion payload inside CodexEvent.
type CodexItem struct {
	Type      string `json:"type"` // "message" or "command".
	Content   string `json:"content,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// CodexProvider implements orchestrator.AgentProvider using Codex CLI.
type CodexProvider struct {
	config   CodexConfig
	wsInfo   WorkspaceResolver
	store    orchestrator.Store
	executor CommandExecutor
	logger   *slog.Logger
}

// NewCodexProvider creates a new CodexProvider.
func NewCodexProvider(config CodexConfig, wsInfo WorkspaceResolver, store orchestrator.Store, executor CommandExecutor, logger *slog.Logger) *CodexProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &CodexProvider{
		config:   config,
		wsInfo:   wsInfo,
		store:    store,
		executor: executor,
		logger:   logger.With(slog.String("component", "codex_provider")),
	}
}

// Name returns the provider's registered name.
func (p *CodexProvider) Name() string {
	return "codex"
}

// Capabilities returns the provider's supported capabilities.
func (p *CodexProvider) Capabilities() []string {
	return []string{"code", "review"}
}

// Execute starts the Codex subprocess to fulfill a task.
func (p *CodexProvider) Execute(ctx context.Context, req orchestrator.ExecuteTaskRequest) (orchestrator.ExecuteTaskResult, error) {
	workDir, err := p.wsInfo.ResolveWorkDir(ctx, req)
	if err != nil {
		return orchestrator.ExecuteTaskResult{Retryable: false}, fmt.Errorf("failed to resolve workspace directory: %w", err)
	}

	// Set up custom environment containing the OpenAI API key.
	env := CodexEnv(p.config)

	buildArgs := func(prompt string) []string {
		return CodexBuildArgs(p.config, prompt)
	}

	parseEvent := CodexParseEvent

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
		BinaryName: "codex",
		BuildArgs:  buildArgs,
		ParseEvent: parseEvent,
		OnEvent:    onEvent,
	}, p.logger)

	prompt := req.Task.Description
	if prompt == "" {
		prompt = req.Task.Title
	}

	output, err := runner.Run(ctx, prompt, workDir, p.executor, env)
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
		Summary:   "Codex execution completed successfully",
		Retryable: false,
	}, nil
}
