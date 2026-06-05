package workspace

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/container"
)

func TestLocalServiceCRUDAndInProcessBridge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	botID := uuid.NewString()
	workspaceRoot := filepath.Join(t.TempDir(), "my-bot")
	svc := NewLocalService(slog.New(slog.DiscardHandler), config.LocalConfig{
		Enabled:                true,
		DefaultWorkspaceParent: t.TempDir(),
		MetadataRoot:           t.TempDir(),
		AllowAbsolutePaths:     true,
	}, t.TempDir())

	info, err := svc.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:         LocalContainerPrefix + botID,
		ImageRef:   "local",
		StorageRef: ctr.StorageRef{Driver: localRuntimeName, Key: workspaceRoot, Kind: "directory"},
		Labels:     map[string]string{BotLabelKey: botID},
	})
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
	if info.StorageRef.Key != workspaceRoot {
		t.Fatalf("workspace path = %q, want %q", info.StorageRef.Key, workspaceRoot)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "IDENTITY.md")); err != nil {
		t.Fatalf("expected seeded bridge template: %v", err)
	}

	if err := svc.StartContainer(ctx, info.ID, nil); err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}
	task, err := svc.GetTaskInfo(ctx, info.ID)
	if err != nil {
		t.Fatalf("GetTaskInfo failed: %v", err)
	}
	if task.Status != ctr.TaskStatusRunning {
		t.Fatalf("task status = %s, want running", task.Status)
	}

	client, err := svc.MCPClient(ctx, botID)
	if err != nil {
		t.Fatalf("MCPClient failed: %v", err)
	}
	realPath := filepath.Join(workspaceRoot, "note.txt")
	if err := client.WriteFile(ctx, realPath, []byte("hello")); err != nil {
		t.Fatalf("WriteFile real path failed: %v", err)
	}
	data, err := os.ReadFile(realPath) //nolint:gosec // test path is under t.TempDir
	if err != nil {
		t.Fatalf("read host file failed: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("host file = %q, want hello", string(data))
	}

	result, err := client.Exec(ctx, "pwd", "", 5)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exec exit = %d, stderr=%s", result.ExitCode, result.Stderr)
	}
	if got := filepath.Clean(strings.TrimSpace(result.Stdout)); got != workspaceRoot {
		t.Fatalf("pwd = %q, want %q", got, workspaceRoot)
	}
}

func TestLocalServiceRefreshesBridgeWhenHostAccessChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	botID := uuid.NewString()
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	approvedRoot := t.TempDir()

	var mu sync.Mutex
	access := WorkspaceHostAccess{}
	resolver := func(context.Context, string) (WorkspaceHostAccess, error) {
		mu.Lock()
		defer mu.Unlock()
		return access, nil
	}

	svc := NewLocalService(slog.New(slog.DiscardHandler), config.LocalConfig{
		Enabled:                true,
		DefaultWorkspaceParent: t.TempDir(),
		MetadataRoot:           t.TempDir(),
		AllowAbsolutePaths:     true,
	}, t.TempDir(), resolver)

	info, err := svc.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:         LocalContainerPrefix + botID,
		ImageRef:   "local",
		StorageRef: ctr.StorageRef{Driver: localRuntimeName, Key: workspaceRoot, Kind: "directory"},
		Labels:     map[string]string{BotLabelKey: botID},
	})
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
	if err := svc.StartContainer(ctx, info.ID, nil); err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}

	client, err := svc.MCPClient(ctx, botID)
	if err != nil {
		t.Fatalf("MCPClient failed: %v", err)
	}
	outsidePath := filepath.Join(approvedRoot, "outside.txt")
	if err := client.WriteFile(ctx, outsidePath, []byte("redirected")); err != nil {
		t.Fatalf("WriteFile before approval failed: %v", err)
	}
	if _, err := os.Stat(outsidePath); !os.IsNotExist(err) {
		t.Fatalf("expected outside path to remain untouched before approval, stat err=%v", err)
	}

	mu.Lock()
	access = WorkspaceHostAccess{
		ApprovedPaths: []WorkspaceApprovedHostPath{{Source: approvedRoot, Status: workspaceApprovedPathStatusApproved}},
	}
	mu.Unlock()

	client, err = svc.MCPClient(ctx, botID)
	if err != nil {
		t.Fatalf("MCPClient after approval failed: %v", err)
	}
	if err := client.WriteFile(ctx, outsidePath, []byte("approved")); err != nil {
		t.Fatalf("WriteFile after approval failed: %v", err)
	}
	got, err := os.ReadFile(outsidePath) //nolint:gosec // test path is under t.TempDir
	if err != nil {
		t.Fatalf("read approved file failed: %v", err)
	}
	if string(got) != "approved" {
		t.Fatalf("approved file = %q, want approved", string(got))
	}
}
