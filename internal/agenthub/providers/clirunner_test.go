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
			if tType == "assistant" {
				tType = "text"
				// Extract "Hello, " or "world!"
				if msg, ok := m["message"].(map[string]any); ok {
					if contentList, ok := msg["content"].([]any); ok && len(contentList) > 0 {
						if firstBlock, ok := contentList[0].(map[string]any); ok {
							content, _ = firstBlock["text"].(string)
						}
					}
				}
			} else if tType == "system" {
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

	output, err := runner.Run(ctx, script, workDir)
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
		BuildArgs: func(prompt string) []string {
			return nil
		},
		ParseEvent: func(line []byte) (CLIEvent, error) {
			return CLIEvent{}, nil
		},
	}

	runner := NewCLIRunner(cfg, slog.Default())
	_, err := runner.Run(context.Background(), "test", "")
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
		ParseEvent: func(line []byte) (CLIEvent, error) {
			return CLIEvent{}, nil
		},
	}

	runner := NewCLIRunner(cfg, slog.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	workDir, err := os.Getwd()
	require.NoError(t, err)

	_, err = runner.Run(ctx, script, workDir)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "signal: killed"))
}

func TestCLIRunner_Run_LargeLine(t *testing.T) {
	// Create a line that is ~130KB.
	largeText := strings.Repeat("A", 130*1024)
	script := fmt.Sprintf("echo '{\"type\":\"text\",\"content\":\"%s\"}'", largeText)

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
			return CLIEvent{
				Type:    m["type"].(string),
				Content: m["content"].(string),
				Raw:     line,
			}, nil
		},
	}

	runner := NewCLIRunner(cfg, slog.Default())
	ctx := context.Background()
	workDir, err := os.Getwd()
	require.NoError(t, err)

	output, err := runner.Run(ctx, script, workDir)
	require.NoError(t, err)
	assert.Equal(t, largeText, output)
}
