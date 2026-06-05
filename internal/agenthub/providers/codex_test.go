package providers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

func TestCodexProvider_Metadata(t *testing.T) {
	p := NewCodexProvider(CodexConfig{}, &mockWorkspaceResolver{}, &mockStore{}, nil, nil)
	assert.Equal(t, "codex", p.Name())
	assert.Contains(t, p.Capabilities(), "code")
	assert.Contains(t, p.Capabilities(), "review")
}

func TestCodexProvider_Execute_Success(t *testing.T) {
	// Create mock 'codex' binary in a temp directory.
	tmpDir, err := os.MkdirTemp("", "mock-codex")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mockBinPath := filepath.Join(tmpDir, "codex")
	mockScript := `#!/bin/bash
echo '{"type":"thread.started"}'
echo '{"type":"item.completed","item":{"type":"message","content":"Codex response content"}}'
echo '{"type":"item.completed","item":{"type":"command","name":"ReadDir"}}'
echo '{"type":"turn.completed"}'
`
	err = os.WriteFile(mockBinPath, []byte(mockScript), 0o755) //nolint:gosec // intentional test executable
	require.NoError(t, err)

	// Prepend tmpDir to PATH.
	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	defer func() { _ = os.Setenv("PATH", oldPath) }()

	store := &mockStore{}
	resolver := &mockWorkspaceResolver{WorkDir: tmpDir}
	p := NewCodexProvider(CodexConfig{
		APIKey:  "test-key-openai",
		Sandbox: "read-only",
	}, resolver, store, nil, nil)

	req := orchestrator.ExecuteTaskRequest{
		Run:       orchestrator.Run{ID: "run-2"},
		Task:      orchestrator.Task{ID: "task-2", Description: "run codex"},
		AttemptNo: 1,
	}

	res, err := p.Execute(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, res.Retryable)
	assert.Contains(t, res.Output["raw_output"].(string), "Codex response content")

	// Verify events were logged to the store.
	require.NotEmpty(t, store.Events)
	var hasOutput, hasToolCall bool
	for _, e := range store.Events {
		if e.Type == orchestrator.EventAgentOutput {
			hasOutput = true
		}
		if e.Type == orchestrator.EventAgentToolCall {
			hasToolCall = true
		}
	}
	assert.True(t, hasOutput)
	assert.True(t, hasToolCall)
}

func TestCodexProvider_Execute_RateLimit(t *testing.T) {
	// Create mock 'codex' binary that exits with status 1 and prints a rate limit error.
	tmpDir, err := os.MkdirTemp("", "mock-codex-err")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mockBinPath := filepath.Join(tmpDir, "codex")
	mockScript := `#!/bin/bash
echo "Rate limit exceeded (429)" >&2
exit 1
`
	err = os.WriteFile(mockBinPath, []byte(mockScript), 0o755) //nolint:gosec // intentional test executable //nolint:gosec // intentional test executable
	require.NoError(t, err)

	// Prepend tmpDir to PATH.
	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	defer func() { _ = os.Setenv("PATH", oldPath) }()

	store := &mockStore{}
	resolver := &mockWorkspaceResolver{WorkDir: tmpDir}
	p := NewCodexProvider(CodexConfig{
		APIKey: "test-key-openai",
	}, resolver, store, nil, nil)

	req := orchestrator.ExecuteTaskRequest{
		Run:  orchestrator.Run{ID: "run-2"},
		Task: orchestrator.Task{ID: "task-2", Description: "trigger rate limit"},
	}

	res, err := p.Execute(context.Background(), req)
	require.Error(t, err)
	assert.True(t, res.Retryable)
	assert.ErrorIs(t, err, ErrRateLimit)
}
