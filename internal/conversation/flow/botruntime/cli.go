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
	// buildStdin, when set, supplies the subprocess stdin (the stream-json
	// current-user message for claudecode). When nil, the turn is driven by a
	// text prompt via promptFor (codex).
	buildStdin   func(in RunInput) string
	parseEvent   func(line []byte) (providers.CLIEvent, error)
	buildEnv     func(in RunInput, creds globalproviders.ModelCredentials) []string
	resolveCreds func(ctx context.Context) (globalproviders.ModelCredentials, error)
	resolver     WorkDirResolver
	executor     ExecutorFactory
	logger       *slog.Logger
}

// NewClaudeCodeRuntime builds the claudecode BotRuntime.
func NewClaudeCodeRuntime(cfg providers.ClaudeCodeConfig, resolveCreds func(ctx context.Context) (globalproviders.ModelCredentials, error), resolver WorkDirResolver, execFac ExecutorFactory, logger *slog.Logger) BotRuntime {
	if logger == nil {
		logger = slog.Default()
	}
	return &cliRuntime{
		name:       bots.FrameworkClaudeCode,
		binaryName: "claude",
		buildArgs: func(in RunInput, _ string) []string {
			localCfg := mergeClaudeCodeConfig(cfg, in.Config.ProviderExt["claudecode"])
			return providers.ClaudeBuildArgsStreamJSON(localCfg, claudeAppendSystem(in, localCfg))
		},
		buildStdin: claudeCurrentUserNDJSON,
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
		resolver:     resolver,
		executor:     execFac,
		logger:       logger.With(slog.String("component", "claudecode_runtime")),
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
		resolver:     resolver,
		executor:     execFac,
		logger:       logger.With(slog.String("component", "codex_runtime")),
	}
}

func (c *cliRuntime) Name() string { return c.name }

func (*cliRuntime) IdleTimeout() time.Duration { return 10 * time.Minute }

// promptFor composes the CLI prompt from the run input. The system preamble is
// prepended when present, followed by any prior conversation history extracted
// from in.Config.Messages, so multi-turn context works with any API backend.
func promptFor(in RunInput) string {
	query := strings.TrimSpace(in.Config.Query)
	system := strings.TrimSpace(in.Config.System)
	history := formatHistory(in.Config.Messages)

	var parts []string
	if system != "" {
		parts = append(parts, system)
	}
	if history != "" {
		parts = append(parts, history)
	}
	if query != "" {
		parts = append(parts, query)
	}
	return strings.Join(parts, "\n\n")
}

// formatHistory formats prior conversation turns (all but the last message,
// which is the current user turn already present in Config.Query) as a plain
// text transcript. Returns empty string when there is no prior history.
func formatHistory(msgs []sdk.Message) string {
	// Need at least 2 messages (one prior turn + current) to have history.
	if len(msgs) < 2 {
		return ""
	}
	prior := msgs[:len(msgs)-1]
	var sb strings.Builder
	sb.WriteString("--- Conversation History ---\n")
	for _, msg := range prior {
		text := extractMsgText(msg)
		if text == "" {
			continue
		}
		switch msg.Role {
		case sdk.MessageRoleUser:
			sb.WriteString("User: ")
		case sdk.MessageRoleAssistant:
			sb.WriteString("Assistant: ")
		default:
			continue
		}
		sb.WriteString(text)
		sb.WriteByte('\n')
	}
	sb.WriteString("--- End of History ---")
	return sb.String()
}

// extractMsgText returns the concatenated text content of an sdk.Message.
func extractMsgText(msg sdk.Message) string {
	var sb strings.Builder
	for _, part := range msg.Content {
		if tp, ok := part.(sdk.TextPart); ok {
			sb.WriteString(tp.Text)
		}
	}
	return strings.TrimSpace(sb.String())
}

// claudeCurrentTurn returns the current user turn text and the prior history for
// the claudecode stream-json path. Non-pipeline turns carry the current message
// in Config.Query (already headerified with the sender name) and Config.Messages
// is pure history. Pipeline turns leave Query empty and the current message is
// the last user entry in Config.Messages, which is then excluded from history.
func claudeCurrentTurn(in RunInput) (current string, history []sdk.Message) {
	msgs := in.Config.Messages
	if q := strings.TrimSpace(in.Config.Query); q != "" {
		return q, msgs
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == sdk.MessageRoleUser {
			rest := make([]sdk.Message, 0, len(msgs)-1)
			rest = append(rest, msgs[:i]...)
			rest = append(rest, msgs[i+1:]...)
			return extractMsgText(msgs[i]), rest
		}
	}
	return "", msgs
}

// claudeAppendSystem builds the --append-system-prompt value: the bot system
// preamble plus a transcript of recent conversation history (bounded by
// MaxContextMessages). History rides here, not as stream-json user turns,
// because the CLI answers every user message it reads from stdin.
func claudeAppendSystem(in RunInput, cfg providers.ClaudeCodeConfig) string {
	system := strings.TrimSpace(in.Config.System)
	_, history := claudeCurrentTurn(in)
	transcript := formatHistoryTranscript(history, cfg.MaxContextMessages)

	var parts []string
	if system != "" {
		parts = append(parts, system)
	}
	if transcript != "" {
		parts = append(parts, transcript)
	}
	return strings.Join(parts, "\n\n")
}

// formatHistoryTranscript renders the last maxMessages history turns as a plain
// text transcript. User turns are written verbatim: the resolver has already
// prefixed them with the sender's "[name] " (see loadMessages) so multiple
// agents in a room stay distinguishable. Assistant turns are labeled
// "Assistant". maxMessages <= 0 means no limit.
func formatHistoryTranscript(msgs []sdk.Message, maxMessages int) string {
	if len(msgs) == 0 {
		return ""
	}
	if maxMessages > 0 && len(msgs) > maxMessages {
		msgs = msgs[len(msgs)-maxMessages:]
	}
	var sb strings.Builder
	sb.WriteString("--- Conversation History ---\n")
	wrote := false
	for _, msg := range msgs {
		text := extractMsgText(msg)
		if text == "" {
			continue
		}
		switch msg.Role {
		case sdk.MessageRoleUser:
			// Text already carries the "[name] " sender prefix from loadMessages.
			sb.WriteString(text)
			sb.WriteByte('\n')
		case sdk.MessageRoleAssistant:
			sb.WriteString("Assistant: ")
			sb.WriteString(text)
			sb.WriteByte('\n')
		default:
			continue
		}
		wrote = true
	}
	if !wrote {
		return ""
	}
	sb.WriteString("--- End of History ---")
	return sb.String()
}

// claudeCurrentUserNDJSON builds the single stream-json user message (one line)
// for the current turn, fed to the CLI via stdin.
func claudeCurrentUserNDJSON(in RunInput) string {
	current, _ := claudeCurrentTurn(in)
	if strings.TrimSpace(current) == "" {
		return ""
	}
	line, err := json.Marshal(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": current},
			},
		},
	})
	if err != nil {
		return ""
	}
	return string(line) + "\n"
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
		var lastThinkingLen int
		var lastTextLen int

		text, err := c.run(ctx, in, func(ev providers.CLIEvent) {
			switch ev.Type {
			case "thinking":
				// Claude's extended thinking content (snapshot mode).
				if len(ev.Content) < lastThinkingLen {
					lastThinkingLen = 0
				}
				if len(ev.Content) > lastThinkingLen {
					delta := ev.Content[lastThinkingLen:]
					send(agentpkg.StreamEvent{Type: agentpkg.EventReasoningDelta, Delta: delta})
					lastThinkingLen = len(ev.Content)
				}
				// Emit visible text that arrived alongside the thinking block.
				if len(ev.TextContent) < lastTextLen {
					lastTextLen = 0
				}
				if len(ev.TextContent) > lastTextLen {
					delta := ev.TextContent[lastTextLen:]
					send(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: delta})
					lastTextLen = len(ev.TextContent)
				}
				if ev.ToolName != "" && activeToolCallID == "" {
					activeToolCallID = uuid.NewString()
					send(agentpkg.StreamEvent{
						Type:       agentpkg.EventToolCallStart,
						ToolName:   ev.ToolName,
						Input:      ev.Payload,
						ToolCallID: activeToolCallID,
					})
				}
			case "text":
				// Claude's visible text reply (snapshot mode).
				if len(ev.Content) < lastTextLen {
					lastTextLen = 0
				}
				if len(ev.Content) > lastTextLen {
					delta := ev.Content[lastTextLen:]
					send(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: delta})
					lastTextLen = len(ev.Content)
				}
				if ev.ToolName != "" && activeToolCallID == "" {
					activeToolCallID = uuid.NewString()
					send(agentpkg.StreamEvent{
						Type:       agentpkg.EventToolCallStart,
						ToolName:   ev.ToolName,
						Input:      ev.Payload,
						ToolCallID: activeToolCallID,
					})
				}
			case "result":
				// Codex: result carries the turn summary — send it.
				// Claudecode stream-json: text was already fully delivered via
				// snapshot "text" events above; sending result again duplicates.
				if ev.Content != "" && c.buildStdin == nil {
					send(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: ev.Content})
				}
			case "tool_use":
				activeToolCallID = uuid.NewString()
				send(agentpkg.StreamEvent{
					Type:       agentpkg.EventToolCallStart,
					ToolName:   ev.Content,
					Input:      ev.Payload,
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

	// Stream-json path (claudecode): the current turn rides on stdin and the
	// CLI prompt arg is empty. Text path (codex): the turn rides in promptFor.
	prompt := ""
	stdin := ""
	if c.buildStdin != nil {
		stdin = c.buildStdin(in)
	} else {
		prompt = promptFor(in)
	}

	if c.buildStdin != nil {
		c.logger.Debug("claudecode stream-json turn",
			slog.Int("history_messages", len(in.Config.Messages)),
			slog.Int("stdin_bytes", len(stdin)),
		)
	}

	runner := providers.NewCLIRunner(providers.CLIRunnerConfig{
		BinaryName: c.binaryName,
		BuildArgs:  func(p string) []string { return c.buildArgs(in, p) },
		ParseEvent: c.parseEvent,
		OnEvent:    onEvent,
		Stdin:      stdin,
	}, c.logger)
	var executor providers.CommandExecutor
	if c.executor != nil {
		executor, err = c.executor(ctx, in.Config.Identity.BotID)
		if err != nil {
			return "", err
		}
	}
	return runner.Run(ctx, prompt, workDir, executor, c.buildEnv(in, creds))
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
	if overlay.MaxContextMessages > 0 {
		base.MaxContextMessages = overlay.MaxContextMessages
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
