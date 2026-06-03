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

func (m *mockWorkspaceResolver) ResolveWorkDir(ctx context.Context, req orchestrator.ExecuteTaskRequest) (string, error) {
	return m.WorkDir, m.Err
}

type mockStore struct {
	Events []orchestrator.RunEvent
	Err    error
}

func (m *mockStore) CreateRun(ctx context.Context, run orchestrator.Run) (orchestrator.Run, error) {
	return run, nil
}
func (m *mockStore) GetRun(ctx context.Context, runID string) (orchestrator.Run, error) {
	return orchestrator.Run{}, nil
}
func (m *mockStore) UpdateRunStatus(ctx context.Context, runID string, status orchestrator.RunStatus) (orchestrator.Run, error) {
	return orchestrator.Run{}, nil
}
func (m *mockStore) ListRunsByStatus(ctx context.Context, statuses ...orchestrator.RunStatus) ([]orchestrator.Run, error) {
	return nil, nil
}
func (m *mockStore) CreateTasks(ctx context.Context, runID string, drafts []orchestrator.TaskDraft) ([]orchestrator.Task, []orchestrator.TaskDependency, error) {
	return nil, nil, nil
}
func (m *mockStore) ListTasks(ctx context.Context, runID string) ([]orchestrator.Task, error) {
	return nil, nil
}
func (m *mockStore) UpdateTaskStatus(ctx context.Context, taskID string, status orchestrator.TaskStatus) (orchestrator.Task, error) {
	return orchestrator.Task{}, nil
}
func (m *mockStore) IncrementTaskAttempt(ctx context.Context, taskID string) (orchestrator.Task, error) {
	return orchestrator.Task{}, nil
}
func (m *mockStore) ListDependencies(ctx context.Context, runID string) ([]orchestrator.TaskDependency, error) {
	return nil, nil
}
func (m *mockStore) CreateAttempt(ctx context.Context, attempt orchestrator.TaskAttempt) (orchestrator.TaskAttempt, error) {
	return attempt, nil
}
func (m *mockStore) CompleteAttempt(ctx context.Context, attemptID string, status orchestrator.AttemptStatus, output map[string]any, errorMessage string, retryable bool) (orchestrator.TaskAttempt, error) {
	return orchestrator.TaskAttempt{}, nil
}
func (m *mockStore) ListAttempts(ctx context.Context, runID string) ([]orchestrator.TaskAttempt, error) {
	return nil, nil
}
func (m *mockStore) AppendEvent(ctx context.Context, event orchestrator.RunEvent) (orchestrator.RunEvent, error) {
	if m.Err != nil {
		return orchestrator.RunEvent{}, m.Err
	}
	m.Events = append(m.Events, event)
	return event, nil
}
func (m *mockStore) ListEvents(ctx context.Context, runID string, afterSeq int64, limit int32) ([]orchestrator.RunEvent, error) {
	return nil, nil
}

func TestClaudeCodeProvider_Metadata(t *testing.T) {
	p := NewClaudeCodeProvider(ClaudeCodeConfig{}, &mockWorkspaceResolver{}, &mockStore{}, nil, nil)
	assert.Equal(t, "claudecode", p.Name())
	assert.Contains(t, p.Capabilities(), "code")
	assert.Contains(t, p.Capabilities(), "review")
}

func TestClaudeCodeProvider_Execute_Success(t *testing.T) {
	// Create mock 'claude' binary in a temp directory.
	tmpDir, err := os.MkdirTemp("", "mock-claude")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mockBinPath := filepath.Join(tmpDir, "claude")
	mockScript := `#!/bin/bash
echo '{"type":"system"}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Claude completed work!"}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"WriteFile"}]}}'
echo '{"type":"result"}'
`
	err = os.WriteFile(mockBinPath, []byte(mockScript), 0755)
	require.NoError(t, err)

	// Prepend tmpDir to PATH.
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	defer os.Setenv("PATH", oldPath)

	store := &mockStore{}
	resolver := &mockWorkspaceResolver{WorkDir: tmpDir}
	p := NewClaudeCodeProvider(ClaudeCodeConfig{
		APIKey:         "test-key",
		PermissionMode: "bypassPermissions",
		MaxTurns:       5,
		AllowedTools:   []string{"WriteFile"},
	}, resolver, store, nil, nil)

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
	defer os.RemoveAll(tmpDir)

	mockBinPath := filepath.Join(tmpDir, "claude")
	mockScript := `#!/bin/bash
echo "Unauthorized authentication signature mismatch" >&2
exit 1
`
	err = os.WriteFile(mockBinPath, []byte(mockScript), 0755)
	require.NoError(t, err)

	// Prepend tmpDir to PATH.
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	defer os.Setenv("PATH", oldPath)

	store := &mockStore{}
	resolver := &mockWorkspaceResolver{WorkDir: tmpDir}
	p := NewClaudeCodeProvider(ClaudeCodeConfig{
		APIKey: "bad-key",
	}, resolver, store, nil, nil)

	req := orchestrator.ExecuteTaskRequest{
		Run:  orchestrator.Run{ID: "run-1"},
		Task: orchestrator.Task{ID: "task-1", Description: "error task"},
	}

	res, err := p.Execute(context.Background(), req)
	assert.Error(t, err)
	assert.False(t, res.Retryable)
	assert.ErrorIs(t, err, ErrAuthFailure)
}
