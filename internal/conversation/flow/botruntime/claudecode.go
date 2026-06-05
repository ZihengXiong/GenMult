package botruntime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/bots"
)

const defaultClaudeBinaryPath = "/opt/memoh/runtime/toolkit/bin/claude"

var (
	ErrClaudeCodeCLINotFound   = errors.New("claude code cli not found")
	ErrClaudeCodeAPIKeyMissing = errors.New("claude code api key not configured")
)

// ClaudeCodeConfig controls how the Claude Code runtime invokes the CLI.
type ClaudeCodeConfig struct {
	APIKey         string
	AuthToken      string
	BaseURL        string
	PermissionMode string
	MaxTurns       int
	Model          string
	BinaryPath     string
}

// ClaudeCodeConfigFromEnv loads runtime defaults from the server environment.
func ClaudeCodeConfigFromEnv() ClaudeCodeConfig {
	cfg := ClaudeCodeConfig{
		APIKey:         strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")),
		AuthToken:      strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN")),
		BaseURL:        strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")),
		PermissionMode: strings.TrimSpace(os.Getenv("CLAUDE_PERMISSION_MODE")),
		Model:          strings.TrimSpace(os.Getenv("CLAUDE_DEFAULT_MODEL")),
		BinaryPath:     strings.TrimSpace(os.Getenv("CLAUDE_CODE_BINARY")),
	}
	if cfg.PermissionMode == "" {
		cfg.PermissionMode = "auto"
	}
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = defaultClaudeBinaryPath
	}
	if raw := strings.TrimSpace(os.Getenv("CLAUDE_MAX_TURNS")); raw != "" {
		if turns, err := strconv.Atoi(raw); err == nil && turns > 0 {
			cfg.MaxTurns = turns
		}
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 15
	}
	return cfg
}

type claudeCodeRuntime struct {
	config ClaudeCodeConfig
	logger *slog.Logger
}

func NewClaudeCodeRuntime(cfg ClaudeCodeConfig, logger *slog.Logger) BotRuntime {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.PermissionMode == "" {
		cfg.PermissionMode = "auto"
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 15
	}
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = defaultClaudeBinaryPath
	}
	return &claudeCodeRuntime{
		config: cfg,
		logger: logger.With(slog.String("component", "claudecode_runtime")),
	}
}

func (*claudeCodeRuntime) Name() string { return bots.FrameworkClaudeCode }

func (*claudeCodeRuntime) IdleTimeout() time.Duration { return 10 * time.Minute }

func (c *claudeCodeRuntime) Stream(ctx context.Context, in RunInput) <-chan agentpkg.StreamEvent {
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
		var sawText bool
		text, err := c.run(ctx, in, func(event claudeCLIEvent) {
			switch event.Type {
			case "text":
				if event.Content == "" {
					return
				}
				sawText = true
				send(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: event.Content})
			case "result":
				if event.Content == "" || sawText {
					return
				}
				sawText = true
				send(agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: event.Content})
			case "tool_use":
				activeToolCallID = uuid.NewString()
				send(agentpkg.StreamEvent{
					Type:       agentpkg.EventToolCallStart,
					ToolName:   event.Content,
					ToolCallID: activeToolCallID,
				})
			case "tool_result":
				send(agentpkg.StreamEvent{
					Type:       agentpkg.EventToolCallEnd,
					Result:     event.Content,
					ToolCallID: activeToolCallID,
				})
				activeToolCallID = ""
			case "error":
				send(agentpkg.StreamEvent{Type: agentpkg.EventError, Error: event.Content})
			}
		})
		if err != nil {
			c.logger.Error("claude code stream failed", slog.Any("error", err), slog.String("bot_id", in.Config.Identity.BotID))
			send(agentpkg.StreamEvent{Type: agentpkg.EventError, Error: err.Error()})
			send(agentpkg.StreamEvent{Type: agentpkg.EventAgentAbort})
			return
		}

		send(terminalEvent(text))
	}()
	return out
}

func (c *claudeCodeRuntime) Generate(ctx context.Context, in RunInput) (*agentpkg.GenerateResult, error) {
	text, err := c.run(ctx, in, nil)
	if err != nil {
		return nil, err
	}
	return &agentpkg.GenerateResult{
		Messages: []sdk.Message{sdk.AssistantMessage(text)},
		Text:     text,
	}, nil
}

func (c *claudeCodeRuntime) run(ctx context.Context, in RunInput, onEvent func(event claudeCLIEvent)) (string, error) {
	cfg := c.config
	if cfg.APIKey == "" && cfg.AuthToken == "" {
		cfg.APIKey = strings.TrimSpace(in.Config.ProviderAPIKey)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = strings.TrimSpace(in.Config.ProviderBaseURL)
	}
	if cfg.Model == "" {
		cfg.Model = strings.TrimSpace(in.Config.ModelID)
	}
	if cfg.APIKey == "" && cfg.AuthToken == "" {
		return "", ErrClaudeCodeAPIKeyMissing
	}

	workDir := filepath.Join(os.TempDir(), "memoh_claudecode", strings.TrimSpace(in.Config.Identity.BotID))
	if err := os.MkdirAll(workDir, 0o755); err != nil { //nolint:gosec // runtime workspace needs execute bit
		return "", fmt.Errorf("prepare claudecode work dir: %w", err)
	}

	return runClaudeCLI(ctx, cfg, promptFor(in), workDir, onEvent, c.logger)
}

func promptFor(in RunInput) string {
	system := strings.TrimSpace(in.Config.System)
	if len(in.Config.Messages) == 0 {
		query := strings.TrimSpace(in.Config.Query)
		if system == "" {
			return query
		}
		return system + "\n\n" + query
	}

	var b strings.Builder
	if system != "" {
		b.WriteString(system)
		b.WriteString("\n\n")
	}
	b.WriteString("Conversation history:\n")
	for _, msg := range in.Config.Messages {
		role := strings.ToUpper(strings.TrimSpace(string(msg.Role)))
		if role == "" {
			role = "USER"
		}
		content := strings.TrimSpace(renderMessageParts(msg.Content))
		if content == "" {
			continue
		}
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func renderMessageParts(parts []sdk.MessagePart) string {
	if len(parts) == 0 {
		return ""
	}
	var rendered []string
	for _, part := range parts {
		switch p := part.(type) {
		case sdk.TextPart:
			if text := strings.TrimSpace(p.Text); text != "" {
				rendered = append(rendered, text)
			}
		case sdk.ReasoningPart:
			if text := strings.TrimSpace(p.Text); text != "" {
				rendered = append(rendered, text)
			}
		case sdk.ToolCallPart:
			rendered = append(rendered, fmt.Sprintf("[tool_call %s]", strings.TrimSpace(p.ToolName)))
		case sdk.ToolResultPart:
			rendered = append(rendered, fmt.Sprintf("[tool_result %s: %v]", strings.TrimSpace(p.ToolName), p.Result))
		case sdk.ImagePart:
			rendered = append(rendered, "[image]")
		case sdk.FilePart:
			name := strings.TrimSpace(p.Filename)
			if name == "" {
				name = "file"
			}
			rendered = append(rendered, fmt.Sprintf("[file %s]", name))
		}
	}
	return strings.Join(rendered, "\n")
}

func claudeEnv(cfg ClaudeCodeConfig) []string {
	var env []string
	thirdParty := cfg.BaseURL != "" && !strings.Contains(cfg.BaseURL, "api.anthropic.com")
	if cfg.AuthToken != "" {
		env = append(env, "ANTHROPIC_AUTH_TOKEN="+cfg.AuthToken)
	} else if cfg.APIKey != "" {
		if thirdParty {
			env = append(env, "ANTHROPIC_AUTH_TOKEN="+cfg.APIKey)
		} else {
			env = append(env, "ANTHROPIC_API_KEY="+cfg.APIKey)
		}
	}
	if cfg.BaseURL != "" {
		env = append(env, "ANTHROPIC_BASE_URL="+cfg.BaseURL)
	}
	return env
}

func claudeBuildArgs(cfg ClaudeCodeConfig, prompt string) []string {
	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
	}
	if cfg.PermissionMode != "" {
		args = append(args, "--permission-mode", cfg.PermissionMode)
	}
	if cfg.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(cfg.MaxTurns))
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	return args
}

type claudeCLIEvent struct {
	Type    string
	Content string
}

type claudeEvent struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content,omitempty"`
	Message *claudeMessage  `json:"message,omitempty"`
	Result  string          `json:"result,omitempty"`
	IsError bool            `json:"is_error,omitempty"`
}

type claudeMessage struct {
	Content []claudeContentBlock `json:"content"`
}

type claudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
}

func parseClaudeEvent(line []byte) (claudeCLIEvent, error) {
	var event claudeEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return claudeCLIEvent{}, err
	}

	switch event.Type {
	case "assistant":
		if event.Message != nil {
			var texts []string
			var tools []string
			for _, block := range event.Message.Content {
				switch block.Type {
				case "text":
					if block.Text != "" {
						texts = append(texts, block.Text)
					}
				case "tool_use":
					if block.Name != "" {
						tools = append(tools, block.Name)
					}
				}
			}
			if len(tools) > 0 {
				return claudeCLIEvent{Type: "tool_use", Content: strings.Join(tools, ", ")}, nil
			}
			if len(texts) > 0 {
				return claudeCLIEvent{Type: "text", Content: strings.Join(texts, "")}, nil
			}
		}
	case "tool_result":
		var result string
		_ = json.Unmarshal(event.Content, &result)
		return claudeCLIEvent{Type: "tool_result", Content: result}, nil
	case "result":
		var result string
		if len(event.Content) > 0 {
			_ = json.Unmarshal(event.Content, &result)
		}
		if result == "" {
			result = event.Result
		}
		if event.IsError {
			return claudeCLIEvent{Type: "error", Content: result}, nil
		}
		return claudeCLIEvent{Type: "result", Content: result}, nil
	case "system":
		return claudeCLIEvent{Type: "init", Content: "initialized"}, nil
	}

	return claudeCLIEvent{Type: event.Type}, nil
}

func runClaudeCLI(
	ctx context.Context,
	cfg ClaudeCodeConfig,
	prompt string,
	workDir string,
	onEvent func(event claudeCLIEvent),
	logger *slog.Logger,
) (string, error) {
	binaryPath, err := exec.LookPath(cfg.BinaryPath)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrClaudeCodeCLINotFound, cfg.BinaryPath)
	}

	cmd := exec.CommandContext(ctx, binaryPath, claudeBuildArgs(cfg, prompt)...) //nolint:gosec // runtime-controlled CLI invocation
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), claudeEnv(cfg)...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var outputBuilder strings.Builder
	var resultFallback string
	var stderrBuf bytes.Buffer
	var stdoutErr error
	var stderrErr error
	var wg sync.WaitGroup
	var mu sync.Mutex

	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			event, err := parseClaudeEvent(line)
			if err != nil {
				if logger != nil {
					logger.Debug("skip unparseable claudecode line", slog.Any("error", err))
				}
				continue
			}
			if onEvent != nil {
				onEvent(event)
			}
			mu.Lock()
			switch event.Type {
			case "text":
				outputBuilder.WriteString(event.Content)
			case "result":
				if strings.TrimSpace(event.Content) != "" && len(event.Content) >= len(resultFallback) {
					resultFallback = event.Content
				}
			}
			mu.Unlock()
		}
		stdoutErr = scanner.Err()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, stderrErr = stderrBuf.ReadFrom(stderr)
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	if stdoutErr != nil {
		return "", stdoutErr
	}
	if stderrErr != nil {
		return "", stderrErr
	}
	if waitErr != nil {
		errText := strings.TrimSpace(stderrBuf.String())
		if errText != "" {
			return "", fmt.Errorf("claudecode failed: %w: %s", waitErr, errText)
		}
		return "", fmt.Errorf("claudecode failed: %w", waitErr)
	}

	output := strings.TrimSpace(outputBuilder.String())
	if output == "" {
		output = strings.TrimSpace(resultFallback)
	}
	return output, nil
}

func terminalEvent(text string) agentpkg.StreamEvent {
	raw, err := json.Marshal([]sdk.Message{sdk.AssistantMessage(text)})
	if err != nil {
		return agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd}
	}
	return agentpkg.StreamEvent{
		Type:     agentpkg.EventAgentEnd,
		Messages: raw,
	}
}
