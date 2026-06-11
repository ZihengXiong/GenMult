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

type mockWorkspaceResolver struct {
	WorkDir string
	Err     error
}

func (m *mockWorkspaceResolver) ResolveWorkDir(_ context.Context, _ orchestrator.ExecuteTaskRequest) (string, error) {
	return m.WorkDir, m.Err
}

type mockStore struct {
	Events []orchestrator.RunEvent
	Err    error
}

func (*mockStore) CreateRun(_ context.Context, run orchestrator.Run) (orchestrator.Run, error) {
	return run, nil
}

func (*mockStore) GetRun(_ context.Context, _ string) (orchestrator.Run, error) {
	return orchestrator.Run{}, nil
}

func (*mockStore) UpdateRunStatus(_ context.Context, _ string, _ orchestrator.RunStatus) (orchestrator.Run, error) {
	return orchestrator.Run{}, nil
}

func (*mockStore) UpdateRunMetadata(_ context.Context, _ string, _ map[string]any) (orchestrator.Run, error) {
	return orchestrator.Run{}, nil
}

func (*mockStore) ListRunsByStatus(_ context.Context, _ ...orchestrator.RunStatus) ([]orchestrator.Run, error) {
	return nil, nil
}

func (*mockStore) GetLatestRunByRoom(_ context.Context, _ string) (orchestrator.Run, error) {
	return orchestrator.Run{}, orchestrator.ErrNotFound
}

func (*mockStore) CreateTasks(_ context.Context, _ string, _ []orchestrator.TaskDraft) ([]orchestrator.Task, []orchestrator.TaskDependency, error) {
	return nil, nil, nil
}

func (*mockStore) ListTasks(_ context.Context, _ string) ([]orchestrator.Task, error) {
	return nil, nil
}

func (*mockStore) UpdateTaskStatus(_ context.Context, _ string, _ orchestrator.TaskStatus) (orchestrator.Task, error) {
	return orchestrator.Task{}, nil
}

func (*mockStore) IncrementTaskAttempt(_ context.Context, _ string) (orchestrator.Task, error) {
	return orchestrator.Task{}, nil
}

func (*mockStore) ListDependencies(_ context.Context, _ string) ([]orchestrator.TaskDependency, error) {
	return nil, nil
}

func (*mockStore) CreateAttempt(_ context.Context, attempt orchestrator.TaskAttempt) (orchestrator.TaskAttempt, error) {
	return attempt, nil
}

func (*mockStore) CompleteAttempt(_ context.Context, _ string, _ orchestrator.AttemptStatus, _ map[string]any, _ string, _ bool) (orchestrator.TaskAttempt, error) {
	return orchestrator.TaskAttempt{}, nil
}

func (*mockStore) ListAttempts(_ context.Context, _ string) ([]orchestrator.TaskAttempt, error) {
	return nil, nil
}

func (m *mockStore) AppendEvent(_ context.Context, event orchestrator.RunEvent) (orchestrator.RunEvent, error) {
	if m.Err != nil {
		return orchestrator.RunEvent{}, m.Err
	}
	m.Events = append(m.Events, event)
	return event, nil
}

func (*mockStore) ListEvents(_ context.Context, _ string, _ int64, _ int32) ([]orchestrator.RunEvent, error) {
	return nil, nil
}

func TestClaudeCodeProvider_Metadata(t *testing.T) {
	p := NewClaudeCodeProvider(ClaudeCodeConfig{}, &mockWorkspaceResolver{}, &mockStore{}, nil, nil, nil)
	assert.Equal(t, "claudecode", p.Name())
	assert.Contains(t, p.Capabilities(), "code")
	assert.Contains(t, p.Capabilities(), "review")
}

func TestClaudeCodeProvider_Execute_Success(t *testing.T) {
	// Create mock 'claude' binary in a temp directory.
	tmpDir, err := os.MkdirTemp("", "mock-claude")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mockBinPath := filepath.Join(tmpDir, "claude")
	mockScript := `#!/bin/bash
echo '{"type":"system"}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Claude completed work!"}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"WriteFile"}]}}'
echo '{"type":"result"}'
`
	err = os.WriteFile(mockBinPath, []byte(mockScript), 0o755) //nolint:gosec // intentional test executable
	require.NoError(t, err)

	// Prepend tmpDir to PATH.
	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	defer func() { _ = os.Setenv("PATH", oldPath) }()

	store := &mockStore{}
	resolver := &mockWorkspaceResolver{WorkDir: tmpDir}
	p := NewClaudeCodeProvider(ClaudeCodeConfig{
		APIKey:         "test-key",
		PermissionMode: "bypassPermissions",
		MaxTurns:       5,
		AllowedTools:   []string{"WriteFile"},
	}, resolver, store, nil, nil, nil)

	req := orchestrator.ExecuteTaskRequest{
		Run:       orchestrator.Run{ID: "run-1"},
		Task:      orchestrator.Task{ID: "task-1", Description: "do something"},
		AttemptNo: 1,
	}

	res, err := p.Execute(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, res.Retryable)
	assert.Contains(t, res.Output["raw_output"].(string), "Claude completed work!")

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

func TestClaudeCodeProvider_Execute_ErrorHandling(t *testing.T) {
	// Create mock 'claude' binary that exits with status 1 and prints an auth error.
	tmpDir, err := os.MkdirTemp("", "mock-claude-err")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mockBinPath := filepath.Join(tmpDir, "claude")
	mockScript := `#!/bin/bash
echo "Unauthorized authentication signature mismatch" >&2
exit 1
`
	err = os.WriteFile(mockBinPath, []byte(mockScript), 0o755) //nolint:gosec // intentional test executable
	require.NoError(t, err)

	// Prepend tmpDir to PATH.
	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	defer func() { _ = os.Setenv("PATH", oldPath) }()

	store := &mockStore{}
	resolver := &mockWorkspaceResolver{WorkDir: tmpDir}
	p := NewClaudeCodeProvider(ClaudeCodeConfig{
		APIKey: "bad-key",
	}, resolver, store, nil, nil, nil)

	req := orchestrator.ExecuteTaskRequest{
		Run:  orchestrator.Run{ID: "run-1"},
		Task: orchestrator.Task{ID: "task-1", Description: "error task"},
	}

	res, err := p.Execute(context.Background(), req)
	require.Error(t, err)
	assert.False(t, res.Retryable)
	assert.ErrorIs(t, err, ErrAuthFailure)
}
