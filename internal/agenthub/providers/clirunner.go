package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// CLIRunnerConfig controls subprocess behavior.
type CLIRunnerConfig struct {
	BinaryName string                              // "claude" or "codex".
	BuildArgs  func(prompt string) []string        // Provider-specific argument construction.
	ParseEvent func(line []byte) (CLIEvent, error) // Provider-specific NDJSON parsing.
	OnEvent    func(event CLIEvent)                // Optional: intermediate event callback.
	Env        []string                            // Optional: environment variables to pass to the command.
}

// CLIEvent is a provider-agnostic intermediate event.
type CLIEvent struct {
	Type    string // "text", "tool_use", "tool_result", "result", "error".
	Content string
	Raw     json.RawMessage
}

// CLIRunner manages subprocess lifecycle.
type CLIRunner struct {
	config CLIRunnerConfig
	logger *slog.Logger
}

// NewCLIRunner creates a new CLIRunner.
func NewCLIRunner(config CLIRunnerConfig, logger *slog.Logger) *CLIRunner {
	if logger == nil {
		logger = slog.Default()
	}
	return &CLIRunner{
		config: config,
		logger: logger.With(slog.String("component", "cli_runner"), slog.String("binary", config.BinaryName)),
	}
}

// Run starts the CLI subprocess and returns the accumulated output.
func (r *CLIRunner) Run(ctx context.Context, prompt string, workDir string) (string, error) {
	// 1. Fail-fast check.
	binaryPath, err := exec.LookPath(r.config.BinaryName)
	if err != nil {
		r.logger.Error("binary not found", slog.String("binary_name", r.config.BinaryName), slog.String("error", err.Error()))
		return "", fmt.Errorf("%w: %s", ErrCLINotFound, r.config.BinaryName)
	}

	// 2. Build arguments.
	args := r.config.BuildArgs(prompt)

	// 3. Create CommandContext.
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Dir = workDir
	if len(r.config.Env) > 0 {
		cmd.Env = r.config.Env
	}

	// 4. Capture Stderr.
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	// 5. Pipe stdout.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	r.logger.Info("starting CLI subprocess",
		slog.String("binary", r.config.BinaryName),
		slog.String("work_dir", workDir),
	)

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	// 6. Read and parse NDJSON output.
	var outputBuilder strings.Builder
	scanner := bufio.NewScanner(stdout)
	// 1MB buffer for potentially long lines.
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	// Start scanning loop.
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		event, err := r.config.ParseEvent(line)
		if err != nil {
			// Skip invalid NDJSON lines gracefully as per plan.
			r.logger.Debug("skipping unparseable line", slog.String("line", string(line)), slog.String("error", err.Error()))
			continue
		}

		// Fire-and-forget event callback.
		if r.config.OnEvent != nil {
			r.config.OnEvent(event)
		}

		// Accumulate text or result content.
		if event.Type == "text" || event.Type == "result" {
			outputBuilder.WriteString(event.Content)
		}
	}

	if err := scanner.Err(); err != nil {
		r.logger.Error("scanner read error", slog.String("error", err.Error()))
	}

	// 7. Wait for subprocess completion.
	err = cmd.Wait()
	stderrStr := stderrBuf.String()

	if err != nil {
		r.logger.Error("CLI command execution failed",
			slog.String("error", err.Error()),
			slog.String("stderr", stderrStr),
		)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// Wrap with stderr details for debugging.
		return "", fmt.Errorf("CLI exit error: %w (stderr: %s)", err, stderrStr)
	}

	return outputBuilder.String(), nil
}
