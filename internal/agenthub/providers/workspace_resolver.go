package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
	"github.com/ZihengXiong/GenMult/internal/workspace"
)

// WorkspaceResolver resolves the subprocess working directory for CLI providers.
type WorkspaceResolver interface {
	// ResolveWorkDir resolves the working directory path for a task execution request.
	ResolveWorkDir(ctx context.Context, req orchestrator.ExecuteTaskRequest) (string, error)
}

// DefaultWorkspaceResolver delegates to workspace.Manager.WorkspaceInfo.
type DefaultWorkspaceResolver struct {
	wsManager *workspace.Manager
}

// NewDefaultWorkspaceResolver creates a new DefaultWorkspaceResolver.
func NewDefaultWorkspaceResolver(wsManager *workspace.Manager) *DefaultWorkspaceResolver {
	return &DefaultWorkspaceResolver{
		wsManager: wsManager,
	}
}

// SharedRoomWorkDir returns the shared host working directory for a room's CLI
// agents. All CLI-framework agents (claudecode/codex) in the same room execute
// in this single directory so files one agent writes are visible to the others
// ("群里 agent 共享一个工作目录"). It must be a host-accessible path because the
// CLI subprocess runs on the server host — the bot's nested-container workspace
// (e.g. /data) is not reachable here, which is why delegating to
// workspace.Manager.WorkspaceInfo would yield an unusable container path.
func SharedRoomWorkDir(roomID string) string {
	return filepath.Join(os.TempDir(), "memoh_agenthub_rooms", strings.TrimSpace(roomID))
}

// ResolveWorkDir resolves the working directory path.
func (r *DefaultWorkspaceResolver) ResolveWorkDir(ctx context.Context, req orchestrator.ExecuteTaskRequest) (string, error) {
	// 1. Explicitly specified workspace path in metadata.
	if req.Task.Metadata != nil {
		if pathVal, ok := req.Task.Metadata["workspace_path"]; ok {
			if pathStr, ok := pathVal.(string); ok && pathStr != "" {
				return pathStr, nil
			}
		}
	}

	// 2. Shared per-room host workdir (created on demand), so every CLI agent in
	//    the room collaborates in the same directory. This is the normal path for
	//    orchestrator runs (which always carry a RoomID).
	if roomID := strings.TrimSpace(req.Run.RoomID); roomID != "" {
		dir := SharedRoomWorkDir(roomID)
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // bot workspace needs exec for shell cmds
			return "", err
		}
		return dir, nil
	}

	// 3. Fallback: delegate to the workspace system by AssignedAgentID (only
	//    reached when no RoomID is present).
	if r.wsManager != nil && req.Task.AssignedAgentID != "" {
		info, err := r.wsManager.WorkspaceInfo(ctx, req.Task.AssignedAgentID)
		if err == nil && info.DefaultWorkDir != "" {
			return info.DefaultWorkDir, nil
		}
	}

	// 4. Fallback to host current working directory.
	return os.Getwd()
}
