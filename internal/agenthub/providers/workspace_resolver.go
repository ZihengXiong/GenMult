package providers

import (
	"context"
	"os"

	"github.com/memohai/memoh/internal/agenthub/orchestrator"
	"github.com/memohai/memoh/internal/workspace"
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

	// 2. Delegate to workspace system using AssignedAgentID.
	if r.wsManager != nil && req.Task.AssignedAgentID != "" {
		info, err := r.wsManager.WorkspaceInfo(ctx, req.Task.AssignedAgentID)
		if err == nil && info.DefaultWorkDir != "" {
			return info.DefaultWorkDir, nil
		}
	}

	// 3. Fallback to host current working directory.
	return os.Getwd()
}
