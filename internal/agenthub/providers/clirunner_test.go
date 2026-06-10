package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLIRunner_Run_Success(t *testing.T) {
	// We use bash to mock a CLI tool printing NDJSON output.
	script := `echo '{"type":"system"}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Hello, "}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"world!"}]}}'
echo '{"type":"result"}'`

	cfg := CLIRunnerConfig{
		BinaryName: "bash",
		BuildArgs: func(prompt string) []string {
			return []string{"-c", prompt}
		},
		ParseEvent: func(line []byte) (CLIEvent, error) {
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				return CLIEvent{}, err
			}
			tType, _ := m["type"].(string)

			// Simple mapping for test.
			var content string
			switch tType {
			case "assistant":
				tType = "text"
				// Extract "Hello, " or "world!"
				if msg, ok := m["message"].(map[string]any); ok {
					if contentList, ok := msg["content"].([]any); ok && len(contentList) > 0 {
						if firstBlock, ok := contentList[0].(map[string]any); ok {
							content, _ = firstBlock["text"].(string)
						}
					}
				}
			case "system":
				tType = "init"
			}
			return CLIEvent{
				Type:    tType,
				Content: content,
				Raw:     line,
			}, nil
		},
	}

	var events []CLIEvent
	cfg.OnEvent = func(e CLIEvent) {
		events = append(events, e)
	}

	runner := NewCLIRunner(cfg, slog.Default())
	ctx := context.Background()
	workDir, err := os.Getwd()
	require.NoError(t, err)

	output, err := runner.Run(ctx, script, workDir, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", output)

	require.Len(t, events, 4)
	assert.Equal(t, "init", events[0].Type)
	assert.Equal(t, "text", events[1].Type)
	assert.Equal(t, "Hello, ", events[1].Content)
	assert.Equal(t, "text", events[2].Type)
	assert.Equal(t, "world!", events[2].Content)
	assert.Equal(t, "result", events[3].Type)
}

func TestCLIRunner_Run_FailFast(t *testing.T) {
	cfg := CLIRunnerConfig{
		BinaryName: "nonexistent-binary-xyz-12345",
		BuildArgs: func(_ string) []string {
			return nil
		},
		ParseEvent: func(_ []byte) (CLIEvent, error) {
			return CLIEvent{}, nil
		},
	}

	runner := NewCLIRunner(cfg, slog.Default())
	_, err := runner.Run(context.Background(), "test", "", nil, nil)
	assert.ErrorIs(t, err, ErrCLINotFound)
}

func TestCLIRunner_Run_Timeout(t *testing.T) {
	// A script that sleeps for 10 seconds.
	script := "sleep 10"

	cfg := CLIRunnerConfig{
		BinaryName: "bash",
		BuildArgs: func(prompt string) []string {
			return []string{"-c", prompt}
		},
		ParseEvent: func(_ []byte) (CLIEvent, error) {
			return CLIEvent{}, nil
		},
	}

	runner := NewCLIRunner(cfg, slog.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	workDir, err := os.Getwd()
	require.NoError(t, err)

	_, err = runner.Run(ctx, script, workDir, nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "signal: killed") || strings.Contains(err.Error(), "context deadline exceeded"))
}

func TestCLIRunner_Run_LargeLine(t *testing.T) {
	// Create a line that is ~130KB.
	largeText := strings.Repeat("A", 130*1024)
	input := fmt.Sprintf("{\"type\":\"text\",\"content\":\"%s\"}\n", largeText)

	cfg := CLIRunnerConfig{
		BinaryName: "cat",
		BuildArgs: func(_ string) []string {
			return nil
		},
		ParseEvent: func(line []byte) (CLIEvent, error) {
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				return CLIEvent{}, err
			}
			return CLIEvent{
				Type:    m["type"].(string),
				Content: m["content"].(string),
				Raw:     line,
			}, nil
		},
		Stdin: input,
	}

	runner := NewCLIRunner(cfg, slog.Default())
	ctx := context.Background()
	workDir, err := os.Getwd()
	require.NoError(t, err)

	output, err := runner.Run(ctx, "", workDir, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, largeText, output)
}

// MockExecHandle is a mock ExecHandle for testing chunk buffering.
type mockExecHandle struct {
	chunks chan ExecChunk
}

func (m *mockExecHandle) Chunks() <-chan ExecChunk {
	return m.chunks
}

func (*mockExecHandle) Wait() (int, error) {
	return 0, nil
}

// MockCommandExecutor is a mock CommandExecutor.
type mockCommandExecutor struct {
	chunks []ExecChunk
}

func (*mockCommandExecutor) LookPath(bin string) (string, error) {
	return "/mock/path/" + bin, nil
}

func (m *mockCommandExecutor) Start(_ context.Context, _ ExecRequest) (ExecHandle, error) {
	ch := make(chan ExecChunk, len(m.chunks)+1)
	for _, chunk := range m.chunks {
		ch <- chunk
	}
	close(ch)
	return &mockExecHandle{chunks: ch}, nil
}

func TestCLIRunner_ChunkBuffering(t *testing.T) {
	// Test streaming chunks that are split in non-newline aligned boundaries.
	chunks := []ExecChunk{
		{Stream: "stdout", Data: []byte(`{"type":"text","content":"He`)},
		{Stream: "stdout", Data: []byte(`llo"}` + "\n" + `{"type":"te` + `xt","content":" `)},
		{Stream: "stdout", Data: []byte(`wor` + `ld"}` + "\n")},
	}

	mockExec := &mockCommandExecutor{chunks: chunks}

	cfg := CLIRunnerConfig{
		BinaryName: "mock-cli",
		BuildArgs: func(_ string) []string {
			return nil
		},
		ParseEvent: func(line []byte) (CLIEvent, error) {
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				return CLIEvent{}, err
			}
			return CLIEvent{
				Type:    m["type"].(string),
				Content: m["content"].(string),
				Raw:     line,
			}, nil
		},
	}

	var events []CLIEvent
	cfg.OnEvent = func(e CLIEvent) {
		events = append(events, e)
	}

	runner := NewCLIRunner(cfg, slog.Default())
	output, err := runner.Run(context.Background(), "test", "", mockExec, nil)
	require.NoError(t, err)

	assert.Equal(t, "Hello world", output)
	require.Len(t, events, 2)
	assert.Equal(t, "Hello", events[0].Content)
	assert.Equal(t, " world", events[1].Content)
}

func TestCLIRunner_ErrorEventEnrichedFromStderr(t *testing.T) {
	// codex emits a JSON error event with no message; the real 401 is on stderr.
	chunks := []ExecChunk{
		{Stream: "stderr", Data: []byte("ERROR codex_api: failed to connect to websocket: HTTP error: 401 Unauthorized\n")},
		{Stream: "stdout", Data: []byte(`{"type":"error"}` + "\n")},
	}

	mockExec := &mockCommandExecutor{chunks: chunks}

	cfg := CLIRunnerConfig{
		BinaryName: "mock-cli",
		BuildArgs:  func(_ string) []string { return nil },
		ParseEvent: func(line []byte) (CLIEvent, error) {
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				return CLIEvent{}, err
			}
			if m["type"].(string) == "error" {
				// Mirror CodexParseEvent's generic fallback.
				return CLIEvent{Type: "error", Content: "unknown codex error", Raw: line}, nil
			}
			return CLIEvent{Type: m["type"].(string), Raw: line}, nil
		},
	}

	var events []CLIEvent
	cfg.OnEvent = func(e CLIEvent) { events = append(events, e) }

	runner := NewCLIRunner(cfg, slog.Default())
	_, err := runner.Run(context.Background(), "test", "", mockExec, nil)
	require.NoError(t, err)

	require.Len(t, events, 1)
	assert.Equal(t, "error", events[0].Type)
	assert.Contains(t, events[0].Content, "401 Unauthorized")
	assert.Contains(t, events[0].Content, "unknown codex error")
}

func TestErrorDetailFromStderr(t *testing.T) {
	assert.Empty(t, errorDetailFromStderr(""))
	assert.Empty(t, errorDetailFromStderr("   \n  \n"))
	// Prefers the error-looking line over a later non-error line.
	got := errorDetailFromStderr("starting up\nERROR 401 Unauthorized\nshutting down")
	assert.Equal(t, "ERROR 401 Unauthorized", got)
	// Falls back to last non-empty line when nothing looks like an error.
	assert.Equal(t, "second", errorDetailFromStderr("first\nsecond\n"))
}

func TestHostExecutor_Success(t *testing.T) {
	executor := NewHostExecutor()
	path, err := executor.LookPath("echo")
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	ctx := context.Background()
	req := ExecRequest{
		Bin:  "echo",
		Args: []string{"hello", "world"},
	}

	handle, err := executor.Start(ctx, req)
	require.NoError(t, err)

	var stdout []byte
	for chunk := range handle.Chunks() {
		if chunk.Stream == "stdout" {
			stdout = append(stdout, chunk.Data...)
		}
	}

	exitCode, err := handle.Wait()
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "hello world\n", string(stdout))
}
