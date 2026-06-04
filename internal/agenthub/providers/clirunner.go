package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// CLIRunnerConfig controls subprocess behavior.
type CLIRunnerConfig struct {
	BinaryName string                              // "claude" or "codex".
	BuildArgs  func(prompt string) []string        // Provider-specific argument construction.
	ParseEvent func(line []byte) (CLIEvent, error) // Provider-specific NDJSON parsing.
	OnEvent    func(event CLIEvent)                // Optional: intermediate event callback.
}

// CLIEvent is a provider-agnostic intermediate event.
type CLIEvent struct {
	Type    string          // "text", "tool_use", "tool_result", "result", "error".
	Content string          // Event-specific main text content.
	Raw     json.RawMessage // Raw JSON output line.
}

// CLIRunner manages subprocess lifecycle and streaming NDJSON parsing.
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

// Run starts the CLI subprocess via the provided CommandExecutor and returns the accumulated output.
func (r *CLIRunner) Run(ctx context.Context, prompt string, workDir string, executor CommandExecutor, env []string) (string, error) {
	if executor == nil {
		executor = NewHostExecutor()
	}

	// 1. Fail-fast check.
	_, err := executor.LookPath(r.config.BinaryName)
	if err != nil {
		r.logger.Error("binary not found via executor", slog.String("binary_name", r.config.BinaryName), slog.String("error", err.Error()))
		return "", fmt.Errorf("%w: %s", ErrCLINotFound, r.config.BinaryName)
	}

	// 2. Build arguments.
	args := r.config.BuildArgs(prompt)

	// 3. Start execution.
	req := ExecRequest{
		Bin:     r.config.BinaryName,
		Args:    args,
		WorkDir: workDir,
		Env:     env,
	}

	handle, err := executor.Start(ctx, req)
	if err != nil {
		r.logger.Error("failed to start execution",
			slog.String("binary", r.config.BinaryName),
			slog.String("work_dir", workDir),
			slog.Any("error", err),
		)
		return "", fmt.Errorf("failed to start execution: %w", err)
	}

	var outputBuilder strings.Builder
	var stderrBuilder strings.Builder
	var lineBuffer bytes.Buffer

	r.logger.Info("started streaming execution",
		slog.String("binary", r.config.BinaryName),
		slog.String("work_dir", workDir),
	)

	// 4. Stream processing loop with cross-chunk buffering.
	for chunk := range handle.Chunks() {
		if chunk.Stream == "stderr" {
			stderrBuilder.Write(chunk.Data)
			continue
		}

		// Handle stdout: buffering and splitting by newline.
		lineBuffer.Write(chunk.Data)
		for {
			data := lineBuffer.Bytes()
			idx := bytes.IndexByte(data, '\n')
			if idx == -1 {
				break
			}

			// Extract line and advance buffer. Copy the slice because
			// lineBuffer.Next invalidates the backing array on future writes.
			line := make([]byte, idx)
			copy(line, data[:idx])
			lineBuffer.Next(idx + 1)

			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}

			event, err := r.config.ParseEvent(line)
			if err != nil {
				// Gracefully skip unparseable lines.
				r.logger.Debug("skipping unparseable line", slog.String("line", string(line)), slog.String("error", err.Error()))
				continue
			}

			if r.config.OnEvent != nil {
				r.config.OnEvent(event)
			}

			// Accumulate text or final result.
			if event.Type == "text" || event.Type == "result" {
				outputBuilder.WriteString(event.Content)
			}
		}
	}

	// Process any leftover content after stream closes.
	if lineBuffer.Len() > 0 {
		line := append([]byte(nil), lineBuffer.Bytes()...)
		if len(bytes.TrimSpace(line)) > 0 {
			event, err := r.config.ParseEvent(line)
			if err == nil {
				if r.config.OnEvent != nil {
					r.config.OnEvent(event)
				}
				if event.Type == "text" || event.Type == "result" {
					outputBuilder.WriteString(event.Content)
				}
			}
		}
	}

	// 5. Wait for process completion.
	exitCode, waitErr := handle.Wait()
	stderrStr := stderrBuilder.String()

	if waitErr != nil {
		r.logger.Error("CLI execution failed",
			slog.Int("exit_code", exitCode),
			slog.String("error", waitErr.Error()),
			slog.String("stderr", stderrStr),
		)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("CLI exit error (code %d): %w (stderr: %s)", exitCode, waitErr, stderrStr)
	}

	return outputBuilder.String(), nil
}
