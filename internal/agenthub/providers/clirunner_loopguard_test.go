package providers

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ZihengXiong/GenMult/internal/agent"
)

// loopTestParseEvent maps {"tool":..., "input":...} test lines to tool_use events.
func loopTestParseEvent(line []byte) (CLIEvent, error) {
	var m struct {
		Tool  string `json:"tool"`
		Input any    `json:"input"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(line, &m); err != nil {
		return CLIEvent{}, err
	}
	if m.Tool != "" {
		return CLIEvent{Type: "tool_use", ToolName: m.Tool, Payload: m.Input, Raw: line}, nil
	}
	return CLIEvent{Type: "text", Content: m.Text, Raw: line}, nil
}

func loopTestConfig() CLIRunnerConfig {
	return CLIRunnerConfig{
		BinaryName: "bash",
		BuildArgs:  func(prompt string) []string { return []string{"-c", prompt} },
		ParseEvent: loopTestParseEvent,
	}
}

// TestCLIRunner_AbortsOnToolLoop feeds a stream of identical tool invocations
// followed by a long sleep: the guard must cancel the subprocess (instead of
// waiting out the sleep) and surface agent.ErrToolLoopDetected.
func TestCLIRunner_AbortsOnToolLoop(t *testing.T) {
	script := `for i in $(seq 1 20); do echo '{"tool":"exec","input":{"cmd":"ls"}}'; done
sleep 30`

	runner := NewCLIRunner(loopTestConfig(), slog.Default())
	workDir, err := os.Getwd()
	require.NoError(t, err)

	start := time.Now()
	_, err = runner.Run(context.Background(), script, workDir, nil, nil)
	require.ErrorIs(t, err, agent.ErrToolLoopDetected)
	assert.Less(t, time.Since(start), 10*time.Second, "subprocess should be killed, not waited out")
}

// TestCLIRunner_NoAbortOnVariedToolCalls runs the same volume of tool calls
// with distinct arguments: no loop, no abort.
func TestCLIRunner_NoAbortOnVariedToolCalls(t *testing.T) {
	script := `for i in $(seq 1 20); do echo "{\"tool\":\"exec\",\"input\":{\"cmd\":\"step-$i\"}}"; done
echo '{"text":"done"}'`

	runner := NewCLIRunner(loopTestConfig(), slog.Default())
	workDir, err := os.Getwd()
	require.NoError(t, err)

	output, err := runner.Run(context.Background(), script, workDir, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", output)
}

// TestCLIRunner_NoAbortWithoutDiscriminator: events with neither tool name nor
// content must not be counted (hashing them would lump unrelated calls).
func TestCLIRunner_NoAbortWithoutDiscriminator(t *testing.T) {
	script := `for i in $(seq 1 20); do echo '{"tool":"","input":null}'; done
echo '{"text":"done"}'`

	cfg := loopTestConfig()
	cfg.ParseEvent = func(line []byte) (CLIEvent, error) {
		var m struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(line, &m); err != nil {
			return CLIEvent{}, err
		}
		if m.Text != "" {
			return CLIEvent{Type: "text", Content: m.Text, Raw: line}, nil
		}
		return CLIEvent{Type: "tool_use", Raw: line}, nil
	}
	runner := NewCLIRunner(cfg, slog.Default())
	workDir, err := os.Getwd()
	require.NoError(t, err)

	output, err := runner.Run(context.Background(), script, workDir, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", output)
}
