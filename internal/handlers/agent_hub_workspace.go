package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/ZihengXiong/GenMult/internal/agenthub/providers"
	"github.com/ZihengXiong/GenMult/internal/auth"
)

// Caps keep these endpoints safe for an in-browser workspace viewer.
const (
	maxWorkspaceFiles    = 500
	maxWorkspaceReadSize = 256 * 1024 // 256 KiB
	maxExecOutput        = 64 * 1024  // 64 KiB
	execTimeout          = 30 * time.Second
)

type workspaceFileEntry struct {
	Path  string `json:"path"` // path relative to the room workspace root
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type listWorkspaceFilesResponse struct {
	Root  string               `json:"root"`
	Items []workspaceFileEntry `json:"items"`
}

type readWorkspaceFileResponse struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

type execWorkspaceRequest struct {
	Command string `json:"command"`
}

type execWorkspaceResponse struct {
	Output    string `json:"output"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated"`
}

// roomWorkspaceDir verifies the caller owns the room and returns its shared host
// workspace directory (created by the CLI providers). Returns an echo error
// response when ownership/lookup fails.
func (h *AgentHubHandler) roomWorkspaceDir(c echo.Context) (string, error) {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	roomID := c.Param("room_id")
	if _, err := h.service.Get(c.Request().Context(), ownerID, roomID); err != nil {
		return "", h.httpError(err)
	}
	return providers.SharedRoomWorkDir(roomID), nil
}

// resolveInWorkspace cleans a user-supplied relative path and guarantees it stays
// inside the workspace root (no traversal escape).
func resolveInWorkspace(root, rel string) (string, bool) {
	clean := filepath.Clean("/" + strings.TrimSpace(rel)) // force-absolute then strip leading slash
	full := filepath.Join(root, strings.TrimPrefix(clean, "/"))
	rootAbs := filepath.Clean(root) + string(os.PathSeparator)
	if full != filepath.Clean(root) && !strings.HasPrefix(full, rootAbs) {
		return "", false
	}
	return full, true
}

// ListWorkspaceFiles godoc
// @Summary List a room's shared workspace files
// @Tags agent-hub
// @Produce json
// @Success 200 {object} listWorkspaceFilesResponse
// @Router /agent-hub/rooms/{room_id}/files [get].
func (h *AgentHubHandler) ListWorkspaceFiles(c echo.Context) error {
	dir, err := h.roomWorkspaceDir(c)
	if err != nil {
		return err
	}
	items := make([]workspaceFileEntry, 0, 64)
	// Best-effort: a missing dir just means no agent has written anything yet.
	// dir is SharedRoomWorkDir(roomID) where roomID was already validated as a
	// UUID the caller owns (roomWorkspaceDir → service.Get), so it can't traverse.
	if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() { //nolint:gosec // dir derived from owner-validated room UUID
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, walkErr error) error { //nolint:gosec // dir derived from owner-validated room UUID
			if walkErr != nil || p == dir {
				return nil //nolint:nilerr // skip unreadable entries, keep walking
			}
			rel, rerr := filepath.Rel(dir, p)
			if rerr != nil {
				return nil
			}
			// Hide VCS internals and node_modules-style noise.
			top := strings.SplitN(rel, string(os.PathSeparator), 2)[0]
			if top == ".git" || top == "node_modules" || top == "target" {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			entry := workspaceFileEntry{Path: filepath.ToSlash(rel), IsDir: d.IsDir()}
			if fi, fierr := d.Info(); fierr == nil {
				entry.Size = fi.Size()
			}
			items = append(items, entry)
			if len(items) >= maxWorkspaceFiles {
				return filepath.SkipAll
			}
			return nil
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return c.JSON(http.StatusOK, listWorkspaceFilesResponse{Root: dir, Items: items})
}

// ReadWorkspaceFile godoc
// @Summary Read a file from a room's shared workspace
// @Tags agent-hub
// @Produce json
// @Param path query string true "Relative file path"
// @Success 200 {object} readWorkspaceFileResponse
// @Router /agent-hub/rooms/{room_id}/files/content [get].
func (h *AgentHubHandler) ReadWorkspaceFile(c echo.Context) error {
	dir, err := h.roomWorkspaceDir(c)
	if err != nil {
		return err
	}
	rel := c.QueryParam("path")
	if strings.TrimSpace(rel) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}
	full, ok := resolveInWorkspace(dir, rel)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid path")
	}
	info, err := os.Stat(full) //nolint:gosec // full validated by resolveInWorkspace (no traversal escape)
	if err != nil || info.IsDir() {
		return echo.NewHTTPError(http.StatusNotFound, "file not found")
	}
	f, err := os.Open(full) //nolint:gosec // path validated by resolveInWorkspace
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "open failed")
	}
	defer func() { _ = f.Close() }()
	// io.ReadAll over a LimitReader, not a single f.Read: Read may return fewer
	// bytes than requested without error, which would silently drop content and
	// mislabel the file as truncated.
	content, err := io.ReadAll(io.LimitReader(f, maxWorkspaceReadSize))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "read failed")
	}
	truncated := info.Size() > int64(len(content))
	return c.JSON(http.StatusOK, readWorkspaceFileResponse{
		Path:      filepath.ToSlash(rel),
		Content:   string(content),
		Truncated: truncated,
	})
}

// ExecWorkspaceCommand godoc
// @Summary Run a shell command in a room's shared workspace
// @Description Runs the command with cwd set to the room's shared workspace dir,
// @Description bounded by a timeout. Scoped to the authenticated owner's room.
// @Tags agent-hub
// @Accept json
// @Produce json
// @Param payload body execWorkspaceRequest true "Command"
// @Success 200 {object} execWorkspaceResponse
// @Router /agent-hub/rooms/{room_id}/exec [post].
func (h *AgentHubHandler) ExecWorkspaceCommand(c echo.Context) error {
	dir, err := h.roomWorkspaceDir(c)
	if err != nil {
		return err
	}
	var req execWorkspaceRequest
	if bindErr := c.Bind(&req); bindErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, bindErr.Error())
	}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "command is required")
	}
	// Ensure the workspace exists so cwd is valid even before any agent ran.
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil { //nolint:gosec // workspace needs exec bit
		return echo.NewHTTPError(http.StatusInternalServerError, "workspace unavailable")
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), execTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command) //nolint:gosec // owner-scoped dev workspace tool
	cmd.Dir = dir
	out, runErr := cmd.CombinedOutput()

	resp := execWorkspaceResponse{}
	if len(out) > maxExecOutput {
		out = out[:maxExecOutput]
		resp.Truncated = true
	}
	resp.Output = string(out)
	if ctx.Err() == context.DeadlineExceeded {
		resp.Output += "\n[命令超时，已终止]"
		resp.ExitCode = -1
	} else if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			resp.ExitCode = exitErr.ExitCode()
		} else {
			resp.ExitCode = -1
			if resp.Output == "" {
				resp.Output = runErr.Error()
			}
		}
	}
	return c.JSON(http.StatusOK, resp)
}
