package providers

// Live test driving the real Claude Code CLI against the DeepSeek
// Anthropic-compatible endpoint — the exact env/args/parse chain the
// orchestrator claudecode provider and the CLI bot runtime use. Triple-gated:
// LIVE_CLI_TEST=1, the `claude` binary on PATH, and ANTHROPIC_AUTH_TOKEN in
// the environment, so `go test ./...` (CI, pre-commit) always skips it. Run:
//
//	set -a; source .env; set +a
//	LIVE_CLI_TEST=1 go test ./internal/agenthub/providers/ -run TestLiveClaudeCLI -v -count=1
//
// Keep the model a cheap flash tier (LIVE_CLI_MODEL or
// ANTHROPIC_DEFAULT_HAIKU_MODEL).

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func liveClaudeConfig(t *testing.T) ClaudeCodeConfig {
	t.Helper()
	if os.Getenv("LIVE_CLI_TEST") == "" {
		t.Skip("live CLI test skipped: set LIVE_CLI_TEST=1 (spawns the real claude CLI, costs tokens)")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("live CLI test skipped: claude binary not on PATH")
	}
	token := os.Getenv("ANTHROPIC_AUTH_TOKEN")
	if token == "" {
		t.Skip("live CLI test skipped: no ANTHROPIC_AUTH_TOKEN in environment")
	}
	model := os.Getenv("LIVE_CLI_MODEL")
	if model == "" {
		model = os.Getenv("ANTHROPIC_DEFAULT_HAIKU_MODEL")
	}
	if model == "" {
		// Cheap flash-tier default, matching the repo .env.
		model = "deepseek-v4-flash"
	}
	cfg := ClaudeCodeConfig{
		AuthToken: token,
		BaseURL:   os.Getenv("ANTHROPIC_BASE_URL"),
		Model:     model,
		MaxTurns:  2,
	}
	t.Logf("live CLI: model=%s base_url=%s", cfg.Model, cfg.BaseURL)
	return cfg
}

// TestLiveClaudeCLIRoundTrip runs one no-tool prompt through the same
// CLIRunner + ClaudeEnv/ClaudeBuildArgs/ClaudeParseEvent chain the providers
// use, against the real CLI and endpoint. It proves credentials, third-party
// auth handling (Bearer token suppressing x-api-key), stream-json parsing,
// and output accumulation end to end.
func TestLiveClaudeCLIRoundTrip(t *testing.T) {
	cfg := liveClaudeConfig(t)

	var events []string
	runner := NewCLIRunner(CLIRunnerConfig{
		BinaryName: "claude",
		BuildArgs:  func(prompt string) []string { return ClaudeBuildArgs(cfg, prompt) },
		ParseEvent: ClaudeParseEvent,
		OnEvent:    func(ev CLIEvent) { events = append(events, ev.Type) },
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	output, err := runner.Run(ctx, "不要使用任何工具，直接回复两个字母：OK", t.TempDir(), nil, ClaudeEnv(cfg))
	if err != nil {
		t.Fatalf("claude CLI run failed: %v (events: %v)", err, events)
	}
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		t.Fatalf("expected non-empty CLI output, events: %v", events)
	}
	// Regression guard: the result event restates the text events' content;
	// accumulating both doubled the reply ("OKOK").
	if strings.Count(strings.ToUpper(trimmed), "OK") > 1 {
		t.Fatalf("reply looks doubled (text + result both accumulated): %q", output)
	}
	t.Logf("claude replied: %q (events: %v)", output, events)
}
