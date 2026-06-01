package botruntime

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agenthub/providers"
	"github.com/memohai/memoh/internal/bots"
)

// WorkDirResolver resolves the host working directory for a bot's CLI runtime.
type WorkDirResolver interface {
	ResolveWorkDir(ctx context.Context, botID string) (string, error)
}

// WorkDirResolverFunc adapts a function to WorkDirResolver.
type WorkDirResolverFunc func(ctx context.Context, botID string) (string, error)

// ResolveWorkDir implements WorkDirResolver.
func (f WorkDirResolverFunc) ResolveWorkDir(ctx context.Context, botID string) (string, error) {
	return f(ctx, botID)
}

// cliRuntime drives a CLI-backed framework (claudecode, codex) for a single
// conversational turn. It reuses providers.CLIRunner plus the shared
// arg/parse/env helpers, and emits agent.StreamEvent so the resolver's existing
// persistence path works unchanged. This is a first-cut runtime: it streams
// assistant text and produces a terminal snapshot; richer tool-call event
// mapping is deferred.
type cliRuntime struct {
	name       string
	binaryName string
	buildArgs  func(prompt string) []string
	parseEvent func(line []byte) (providers.CLIEvent, error)
	env        []string
	resolver   WorkDirResolver
	logger     *slog.Logger
}

// NewClaudeCodeRuntime builds the claudecode BotRuntime.
func NewClaudeCodeRuntime(cfg providers.ClaudeCodeConfig, resolver WorkDirResolver, logger *slog.Logger) BotRuntime {
	if logger == nil {
		logger = slog.Default()
	}
	return &cliRuntime{
		name:       bots.FrameworkClaudeCode,
		binaryName: "claude",
		buildArgs:  func(prompt string) []string { return providers.ClaudeBuildArgs(cfg, prompt) },
		parseEvent: providers.ClaudeParseEvent,
		env:        providers.ClaudeEnv(cfg),
		resolver:   resolver,
		logger:     logger.With(slog.String("component", "claudecode_runtime")),
	}
}

// NewCodexRuntime builds the codex BotRuntime.
func NewCodexRuntime(cfg providers.CodexConfig, resolver WorkDirResolver, logger *slog.Logger) BotRuntime {
	if logger == nil {
		logger = slog.Default()
	}
	return &cliRuntime{
		name:       bots.FrameworkCodex,
		binaryName: "codex",
		buildArgs:  func(prompt string) []string { return providers.CodexBuildArgs(cfg, prompt) },
		parseEvent: providers.CodexParseEvent,
		env:        providers.CodexEnv(cfg),
		resolver:   resolver,
		logger:     logger.With(slog.String("component", "codex_runtime")),
	}
}

func (c *cliRuntime) Name() string { return c.name }

// promptFor composes the CLI prompt from the run input. The system preamble is
// prepended when present so the framework has the same high-level instructions
// the memoh agent would receive.
func promptFor(in RunInput) string {
	query := strings.TrimSpace(in.Config.Query)
	system := strings.TrimSpace(in.Config.System)
	if system == "" {
		return query
	}
	return system + "\n\n" + query
}

func (c *cliRuntime) Stream(ctx context.Context, in RunInput) <-chan agentpkg.StreamEvent {
	out := make(chan agentpkg.StreamEvent)
	go func() {
		defer close(out)

		send := func(ev agentpkg.StreamEvent) bool {
			select {
			case out <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}

		send(agentpkg.StreamEvent{Type: agentpkg.EventAgentStart})

		text, err := c.run(ctx, in, func(ev providers.CLIEvent) {
			if ev.Type == "text" && ev.Content != "" {
				send(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: ev.Content})
			}
		})
		if err != nil {
			c.logger.Error("cli runtime stream failed", slog.String("error", err.Error()))
			send(agentpkg.StreamEvent{Type: agentpkg.EventError, Error: err.Error()})
			send(agentpkg.StreamEvent{Type: agentpkg.EventAgentAbort})
			return
		}

		send(terminalEvent(text))
	}()
	return out
}

func (c *cliRuntime) Generate(ctx context.Context, in RunInput) (*agentpkg.GenerateResult, error) {
	text, err := c.run(ctx, in, nil)
	if err != nil {
		return nil, err
	}
	return &agentpkg.GenerateResult{
		Messages: []sdk.Message{sdk.AssistantMessage(text)},
		Text:     text,
	}, nil
}

// run resolves the work dir and executes the CLI, returning the accumulated
// assistant text.
func (c *cliRuntime) run(ctx context.Context, in RunInput, onEvent func(providers.CLIEvent)) (string, error) {
	workDir, err := c.resolver.ResolveWorkDir(ctx, in.Config.Identity.BotID)
	if err != nil {
		return "", err
	}
	runner := providers.NewCLIRunner(providers.CLIRunnerConfig{
		BinaryName: c.binaryName,
		BuildArgs:  c.buildArgs,
		ParseEvent: c.parseEvent,
		OnEvent:    onEvent,
	}, c.logger)
	return runner.Run(ctx, promptFor(in), workDir, nil, c.env)
}

// terminalEvent builds the terminal agent_end event carrying the assistant
// message snapshot so the resolver can persist the turn.
func terminalEvent(text string) agentpkg.StreamEvent {
	msgs := []sdk.Message{sdk.AssistantMessage(text)}
	raw, err := json.Marshal(msgs)
	if err != nil {
		return agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd}
	}
	return agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd, Messages: raw}
}
