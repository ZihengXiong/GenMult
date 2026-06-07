package workspace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	ctr "github.com/ZihengXiong/GenMult/internal/container"
	"github.com/ZihengXiong/GenMult/internal/db"
	dbstore "github.com/ZihengXiong/GenMult/internal/db/store"
	"github.com/ZihengXiong/GenMult/internal/workspace/bridge"
)

const (
	workspaceHostAccessMetadataKey      = "host_access"
	workspaceTrustedRootMetadataKey     = "trusted_root"
	workspaceApprovedPathsMetadataKey   = "approved_paths"
	workspaceApprovedPathSourceKey      = "source"
	workspaceApprovedPathStatusKey      = "status"
	workspaceApprovedPathStatusApproved = "approved"
	workspaceHostMountBasePath          = "/mounts"
)

type HostAccessResolver func(ctx context.Context, botID string) (WorkspaceHostAccess, error)

type WorkspaceHostAccess struct {
	Backend       string                      `json:"backend,omitempty"`
	WorkspaceRoot string                      `json:"workspace_root,omitempty"`
	TrustedRoot   string                      `json:"trusted_root,omitempty"`
	ApprovedPaths []WorkspaceApprovedHostPath `json:"approved_paths,omitempty"`
}

type WorkspaceApprovedHostPath struct {
	Source string `json:"source,omitempty"`
	Status string `json:"status,omitempty"`
}

func NewBotHostAccessResolver(queries dbstore.Queries) HostAccessResolver {
	if queries == nil {
		return nil
	}
	return func(ctx context.Context, botID string) (WorkspaceHostAccess, error) {
		botUUID, err := db.ParseUUID(botID)
		if err != nil {
			return WorkspaceHostAccess{}, err
		}
		row, err := queries.GetBotByID(ctx, botUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return WorkspaceHostAccess{}, nil
			}
			return WorkspaceHostAccess{}, err
		}
		metadata, err := decodeBotMetadata(row.Metadata)
		if err != nil {
			return WorkspaceHostAccess{}, err
		}
		return workspaceHostAccessFromMetadata(metadata), nil
	}
}

func workspaceHostAccessFromMetadata(metadata map[string]any) WorkspaceHostAccess {
	section := workspaceSection(metadata)
	raw, ok := section[workspaceHostAccessMetadataKey]
	if !ok {
		return WorkspaceHostAccess{}
	}
	hostAccessSection, ok := raw.(map[string]any)
	if !ok {
		return WorkspaceHostAccess{}
	}

	access := WorkspaceHostAccess{
		Backend:       workspaceBackendFromMetadata(metadata),
		WorkspaceRoot: strings.TrimSpace(localWorkspacePathFromMetadata(metadata)),
		TrustedRoot:   strings.TrimSpace(readAnyString(hostAccessSection[workspaceTrustedRootMetadataKey])),
	}

	switch typed := hostAccessSection[workspaceApprovedPathsMetadataKey].(type) {
	case []any:
		access.ApprovedPaths = parseApprovedHostPaths(typed)
	case []map[string]any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		access.ApprovedPaths = parseApprovedHostPaths(items)
	}

	return access
}

func parseApprovedHostPaths(items []any) []WorkspaceApprovedHostPath {
	if len(items) == 0 {
		return nil
	}
	out := make([]WorkspaceApprovedHostPath, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path := WorkspaceApprovedHostPath{
			Source: strings.TrimSpace(readAnyString(m[workspaceApprovedPathSourceKey])),
			Status: strings.TrimSpace(readAnyString(m[workspaceApprovedPathStatusKey])),
		}
		if path.Source == "" {
			continue
		}
		out = append(out, path)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func readAnyString(value any) string {
	text, _ := value.(string)
	return text
}

func (a WorkspaceHostAccess) AllowedHostPaths() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 1+len(a.ApprovedPaths))
	add := func(raw string) {
		path := normalizeHostAccessPath(raw)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}

	if a.TrustedRoot != "" {
		add(a.TrustedRoot)
	} else {
		add(a.WorkspaceRoot)
	}
	for _, approved := range a.ApprovedPaths {
		if approved.Status != "" && !strings.EqualFold(strings.TrimSpace(approved.Status), workspaceApprovedPathStatusApproved) {
			continue
		}
		add(approved.Source)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (a WorkspaceHostAccess) IsLocalWorkspace() bool {
	return strings.EqualFold(strings.TrimSpace(a.Backend), bridge.WorkspaceBackendLocal)
}

func (a WorkspaceHostAccess) ContainerMounts() []ctr.MountSpec {
	if a.IsLocalWorkspace() {
		return nil
	}
	allowed := a.AllowedHostPaths()
	if len(allowed) == 0 {
		return nil
	}
	mounts := make([]ctr.MountSpec, 0, len(allowed))
	for idx, path := range allowed {
		mounts = append(mounts, ctr.MountSpec{
			Destination: fmt.Sprintf("%s/host-%d", workspaceHostMountBasePath, idx),
			Type:        "bind",
			Source:      path,
			Options:     []string{"rbind", "rw"},
		})
	}
	return mounts
}

func normalizeHostAccessPath(raw string) string {
	path := strings.TrimSpace(raw)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func NormalizeApprovedHostPath(raw string) string {
	return normalizeHostAccessPath(raw)
}

func WithApprovedHostPath(metadata map[string]any, source string) map[string]any {
	source = normalizeHostAccessPath(source)
	if source == "" {
		return cloneAnyMap(metadata)
	}

	next := cloneAnyMap(metadata)
	section := workspaceSection(next)
	hostAccessSection, _ := section[workspaceHostAccessMetadataKey].(map[string]any)
	hostAccessSection = cloneAnyMap(hostAccessSection)

	var existing []any
	if typed, ok := hostAccessSection[workspaceApprovedPathsMetadataKey].([]any); ok {
		existing = append(existing, typed...)
	}

	alreadyPresent := false
	normalized := make([]any, 0, len(existing)+1)
	for _, item := range existing {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rawSource := normalizeHostAccessPath(readAnyString(entry[workspaceApprovedPathSourceKey]))
		if rawSource == "" {
			continue
		}
		status := strings.TrimSpace(readAnyString(entry[workspaceApprovedPathStatusKey]))
		if rawSource == source && (status == "" || strings.EqualFold(status, workspaceApprovedPathStatusApproved)) {
			alreadyPresent = true
		}
		normalized = append(normalized, map[string]any{
			workspaceApprovedPathSourceKey: rawSource,
			workspaceApprovedPathStatusKey: firstNonEmptyString(status, workspaceApprovedPathStatusApproved),
		})
	}

	if !alreadyPresent {
		normalized = append(normalized, map[string]any{
			workspaceApprovedPathSourceKey: source,
			workspaceApprovedPathStatusKey: workspaceApprovedPathStatusApproved,
		})
	}

	hostAccessSection[workspaceApprovedPathsMetadataKey] = normalized
	section[workspaceHostAccessMetadataKey] = hostAccessSection
	next[workspaceMetadataKey] = section
	return next
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
