package botruntime

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/ZihengXiong/GenMult/internal/agent"
	"github.com/ZihengXiong/GenMult/internal/agenthub/providers"
	"github.com/ZihengXiong/GenMult/internal/bots"
	globalproviders "github.com/ZihengXiong/GenMult/internal/providers"
)

// WorkDirResolver resolves the host working directory for a bot's CLI runtime.
type WorkDirResolver interface {
	ResolveWorkDir(ctx context.Context, botID string) (string, error)
}

// WorkDirResolverFunc adapts a function to WorkDirResolver.
type WorkDirResolverFunc func(ctx context.Context, botID string) (string, error)

// ExecutorFactory creates a CommandExecutor for a specific bot's context.
type ExecutorFactory func(ctx context.Context, botID string) (providers.CommandExecutor, error)

// ResolveWorkDir implements WorkDirResolver.
func (f WorkDirResolverFunc) ResolveWorkDir(ctx context.Context, botID string) (string, error) {
	return f(ctx, botID)
}

// cliRuntime drives a CLI-backed framework (claudecode, codex) for a single
// conversational turn. It reuses providers.CLIRunner plus the shared
// arg/parse/env helpers, and emits agent.StreamEvent so the resolver's existing
// persistence path works unchanged.
type cliRuntime struct {
	name       string
	binaryName string
	buildArgs  func(in RunInput, prompt string) []string
	parseEvent func(line []byte) (providers.CLIEvent, error)
	buildEnv   func(in RunInput, creds globalproviders.ModelCredentials) []string
	resolveCreds func(ctx context.Context) (globalproviders.ModelCredentials, error)
	resolver   WorkDirResolver
	executor   ExecutorFactory
	logger     *slog.Logger
}

// NewClaudeCodeRuntime builds the claudecode BotRuntime.
func NewClaudeCodeRuntime(cfg providers.ClaudeCodeConfig, resolveCreds func(ctx context.Context) (globalproviders.ModelCredentials, error), resolver WorkDirResolver, execFac ExecutorFactory, logger *slog.Logger) BotRuntime {
	if logger == nil {
		logger = slog.Default()
	}
	return &cliRuntime{
		name:       bots.FrameworkClaudeCode,
		binaryName: "claude",
		buildArgs: func(in RunInput, prompt string) []string {
			localCfg := mergeClaudeCodeConfig(cfg, in.Config.ProviderExt["claudecode"])
			return providers.ClaudeBuildArgs(localCfg, prompt)
		},
		parseEvent: providers.ClaudeParseEvent,
		buildEnv: func(in RunInput, creds globalproviders.ModelCredentials) []string {
			localCfg := mergeClaudeCodeConfig(cfg, in.Config.ProviderExt["claudecode"])
			if creds.APIKey != "" && localCfg.APIKey == "" && localCfg.AuthToken == "" {
				localCfg.APIKey = creds.APIKey
			}
			if creds.BaseURL != "" && localCfg.BaseURL == "" {
				localCfg.BaseURL = creds.BaseURL
			}
			return providers.ClaudeEnv(localCfg)
		},
		resolveCreds: resolveCreds,
		resolver:   resolver,
		executor:   execFac,
		logger:     logger.With(slog.String("component", "claudecode_runtime")),
	}
}

// NewCodexRuntime builds the codex BotRuntime.
func NewCodexRuntime(cfg providers.CodexConfig, resolveCreds func(ctx context.Context) (globalproviders.ModelCredentials, error), resolver WorkDirResolver, execFac ExecutorFactory, logger *slog.Logger) BotRuntime {
	if logger == nil {
		logger = slog.Default()
	}
	return &cliRuntime{
		name:       bots.FrameworkCodex,
		binaryName: "codex",
		buildArgs: func(in RunInput, prompt string) []string {
			localCfg := mergeCodexConfig(cfg, in.Config.ProviderExt["codex"])
			return providers.CodexBuildArgs(localCfg, prompt)
		},
		parseEvent: providers.CodexParseEvent,
		buildEnv: func(in RunInput, creds globalproviders.ModelCredentials) []string {
			localCfg := mergeCodexConfig(cfg, in.Config.ProviderExt["codex"])
			if creds.APIKey != "" && localCfg.APIKey == "" {
				localCfg.APIKey = creds.APIKey
			}
			if creds.BaseURL != "" && localCfg.BaseURL == "" {
				localCfg.BaseURL = creds.BaseURL
			}
			return providers.CodexEnv(localCfg)
		},
		resolveCreds: resolveCreds,
		resolver:   resolver,
		executor:   execFac,
		logger:     logger.With(slog.String("component", "codex_runtime")),
	}
}

func (c *cliRuntime) Name() string { return c.name }

func (*cliRuntime) IdleTimeout() time.Duration { return 10 * time.Minute }

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

		var activeToolCallID string

		text, err := c.run(ctx, in, func(ev providers.CLIEvent) {
			switch ev.Type {
			case "text", "result":
				if ev.Content != "" {
					send(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: ev.Content})
				}
			case "tool_use":
				activeToolCallID = uuid.NewString()
				send(agentpkg.StreamEvent{
					Type:       agentpkg.EventToolCallStart,
					ToolName:   ev.Content,
					ToolCallID: activeToolCallID,
				})
			case "tool_result":
				send(agentpkg.StreamEvent{
					Type:       agentpkg.EventToolCallEnd,
					Result:     ev.Content,
					ToolCallID: activeToolCallID,
				})
				activeToolCallID = ""
			case "error":
				send(agentpkg.StreamEvent{Type: agentpkg.EventError, Error: ev.Content})
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
	creds, err := c.resolveCreds(ctx)
	if err != nil {
		return "", err
	}
	runner := providers.NewCLIRunner(providers.CLIRunnerConfig{
		BinaryName: c.binaryName,
		BuildArgs:  func(prompt string) []string { return c.buildArgs(in, prompt) },
		ParseEvent: c.parseEvent,
		OnEvent:    onEvent,
	}, c.logger)
	var executor providers.CommandExecutor
	if c.executor != nil {
		executor, err = c.executor(ctx, in.Config.Identity.BotID)
		if err != nil {
			return "", err
		}
	}
	return runner.Run(ctx, promptFor(in), workDir, executor, c.buildEnv(in, creds))
}

func mergeClaudeCodeConfig(base providers.ClaudeCodeConfig, ext any) providers.ClaudeCodeConfig {
	if ext == nil {
		return base
	}
	b, err := json.Marshal(ext)
	if err != nil {
		return base
	}
	var overlay providers.ClaudeCodeConfig
	if err := json.Unmarshal(b, &overlay); err != nil {
		return base
	}
	if overlay.Model != "" {
		base.Model = overlay.Model
	}
	if overlay.APIKey != "" {
		base.APIKey = overlay.APIKey
	}
	if overlay.AuthToken != "" {
		base.AuthToken = overlay.AuthToken
	}
	if overlay.BaseURL != "" {
		base.BaseURL = overlay.BaseURL
	}
	if overlay.PermissionMode != "" {
		base.PermissionMode = overlay.PermissionMode
	}
	if overlay.MaxTurns > 0 {
		base.MaxTurns = overlay.MaxTurns
	}
	if len(overlay.AllowedTools) > 0 {
		base.AllowedTools = overlay.AllowedTools
	}
	if len(overlay.CustomEnv) > 0 {
		if base.CustomEnv == nil {
			base.CustomEnv = make(map[string]string)
		}
		for k, v := range overlay.CustomEnv {
			base.CustomEnv[k] = v
		}
	}
	return base
}

func mergeCodexConfig(base providers.CodexConfig, ext any) providers.CodexConfig {
	if ext == nil {
		return base
	}
	b, err := json.Marshal(ext)
	if err != nil {
		return base
	}
	var overlay providers.CodexConfig
	if err := json.Unmarshal(b, &overlay); err != nil {
		return base
	}
	if overlay.Model != "" {
		base.Model = overlay.Model
	}
	if overlay.Sandbox != "" {
		base.Sandbox = overlay.Sandbox
	}

	return base
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
