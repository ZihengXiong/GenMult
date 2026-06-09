package providers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

// ErrMemohRunnerMissing is returned when a memoh task is dispatched but no
// MemohRunner was wired (e.g. the chat resolver is unavailable). The task fails
// explicitly instead of silently no-op succeeding.
var ErrMemohRunnerMissing = errors.New("memoh runner not configured")

// MemohRunner runs a single headless turn of a built-in (memoh framework) bot
// and returns its final assistant text. It is the seam that lets the
// orchestrator drive a memoh bot without the agenthub packages importing
// internal/conversation/flow (which would create an import cycle:
// flow -> flow/botruntime -> agenthub/providers). The concrete implementation
// lives at the composition root (cmd/agent), wrapping *flow.Resolver.Chat.
type MemohRunner interface {
	RunTurn(ctx context.Context, in RunTurnInput) (RunTurnResult, error)
}

// RunTurnInput is the minimal request to run one memoh bot turn.
type RunTurnInput struct {
	BotID     string // bare bot UUID (no "bot:" prefix)
	UserID    string // owner user id, for model-credential / settings resolution
	SessionID string // isolates orchestrator output from the user's direct chat
	Prompt    string // the task prompt (already includes room history context)
}

// RunTurnResult carries the final assistant text of a memoh turn.
type RunTurnResult struct {
	Text string
}

// MemohProvider implements orchestrator.AgentProvider for built-in memoh bots
// (e.g. a DeepSeek-backed bot). Unlike the CLI providers it runs no subprocess;
// it delegates to a MemohRunner that reuses the existing chat resolver.
type MemohProvider struct {
	runner MemohRunner
	store  orchestrator.Store
	logger *slog.Logger
}

// NewMemohProvider creates a MemohProvider. runner may be nil, in which case
// Execute returns ErrMemohRunnerMissing (non-retryable) so the failure is
// visible rather than a silent noop success.
func NewMemohProvider(runner MemohRunner, store orchestrator.Store, logger *slog.Logger) *MemohProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &MemohProvider{
		runner: runner,
		store:  store,
		logger: logger.With(slog.String("component", "memoh_provider")),
	}
}

// Name returns the provider's registered name.
func (*MemohProvider) Name() string {
	return "memoh"
}

// Capabilities returns the provider's supported capabilities. Memoh bots are
// chat/reasoning agents (plan, analysis, review) rather than CLI code runners.
func (*MemohProvider) Capabilities() []string {
	return []string{"plan", "analysis", "review", "chat"}
}

// Execute runs one memoh bot turn for the task and records its output as a run
// event. The bot runs in a per-run session ("agenthub-run-<runID>") so its
// orchestrator output never lands in the user's direct chat history.
func (p *MemohProvider) Execute(ctx context.Context, req orchestrator.ExecuteTaskRequest) (orchestrator.ExecuteTaskResult, error) {
	if p.runner == nil {
		return orchestrator.ExecuteTaskResult{Retryable: false}, ErrMemohRunnerMissing
	}

	botID := normalizeBotID(req.Task.AssignedAgentID)
	if botID == "" {
		return orchestrator.ExecuteTaskResult{Retryable: false}, fmt.Errorf("memoh task %s has no assigned bot id", req.Task.ID)
	}

	prompt := PromptWithContext(req)
	// The run id is itself a UUID and unique per run; the chat resolver parses
	// SessionID as a UUID, so reuse it directly. This gives every orchestrator
	// run its own isolated chat session (multi-turn context shared across the
	// run's memoh tasks) without colliding with the user's direct chat sessions.
	sessionID := strings.TrimSpace(req.Run.ID)

	p.logger.Info("starting memoh task execution",
		slog.String("task_id", req.Task.ID),
		slog.String("run_id", req.Run.ID),
		slog.String("bot_id", botID),
	)

	result, err := p.runner.RunTurn(ctx, RunTurnInput{
		BotID:     botID,
		UserID:    strings.TrimSpace(req.Run.CreatedBy),
		SessionID: sessionID,
		Prompt:    prompt,
	})
	if err != nil {
		p.logger.Error("memoh execution failed",
			slog.String("task_id", req.Task.ID),
			slog.String("run_id", req.Run.ID),
			slog.Any("error", err),
		)
		// Conservatively retryable: chat-resolver failures are often transient
		// (model rate limits, timeouts). The orchestrator caps attempts.
		return orchestrator.ExecuteTaskResult{Retryable: true}, err
	}

	text := strings.TrimSpace(result.Text)
	if _, err := p.store.AppendEvent(ctx, orchestrator.RunEvent{
		ID:        uuid.NewString(),
		RunID:     req.Run.ID,
		TaskID:    req.Task.ID,
		Type:      orchestrator.EventAgentOutput,
		Payload:   map[string]any{"content": text, "raw_type": "text"},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		p.logger.Error("failed to append memoh agent event to store", slog.String("error", err.Error()))
	}

	p.logger.Info("memoh task execution completed successfully",
		slog.String("task_id", req.Task.ID),
		slog.String("run_id", req.Run.ID),
	)

	return orchestrator.ExecuteTaskResult{
		Output:    map[string]any{"raw_output": text},
		Summary:   "Memoh agent reply generated",
		Retryable: false,
	}, nil
}

// normalizeBotID strips the "bot:" prefix that room agent ids carry, returning
// the bare UUID the chat resolver expects. The frontend's buildRunAgents sends
// the bare botId, but agentsFromRoom-derived runs carry "bot:UUID" — handle both.
func normalizeBotID(agentID string) string {
	id := strings.TrimSpace(agentID)
	return strings.TrimPrefix(id, "bot:")
}
