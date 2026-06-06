package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/ZihengXiong/GenMult/internal/boot"
	"github.com/ZihengXiong/GenMult/internal/config"
	ctr "github.com/ZihengXiong/GenMult/internal/container"
	"github.com/ZihengXiong/GenMult/internal/version"
)

type PingResponse struct {
	Status                string `json:"status"`
	ContainerBackend      string `json:"container_backend"`
	LocalWorkspaceEnabled bool   `json:"local_workspace_enabled"`
	SnapshotSupported     bool   `json:"snapshot_supported"`
	Version               string `json:"version"`
	CommitHash            string `json:"commit_hash"`
}

type ValidateDirectoryRequest struct {
	Path string `json:"path"`
}

type ValidateDirectoryResponse struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

type ListDirectoryRequest struct {
	Path string `json:"path"`
}

type ListDirectoryEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

type ListDirectoryResponse struct {
	Path    string               `json:"path"`
	Parent  string               `json:"parent,omitempty"`
	Entries []ListDirectoryEntry `json:"entries"`
}

type PingHandler struct {
	logger  *slog.Logger
	runtime *boot.RuntimeConfig
	service ctr.Service
	cfg     config.Config
}

type snapshotCapabilityProvider interface {
	SnapshotSupported(ctx context.Context) bool
}

func NewPingHandler(log *slog.Logger, rc *boot.RuntimeConfig, service ctr.Service, cfg config.Config) *PingHandler {
	return &PingHandler{
		logger:  log.With(slog.String("handler", "ping")),
		runtime: rc,
		service: service,
		cfg:     cfg,
	}
}

func (h *PingHandler) Register(e *echo.Echo) {
	e.GET("/ping", h.Ping)
	e.HEAD("/health", h.PingHead)
	e.POST("/system/validate-directory", h.ValidateDirectory)
	e.POST("/system/list-directory", h.ListDirectory)
}

// Ping godoc
// @Summary Health check with server capabilities
// @Tags system
// @Success 200 {object} PingResponse
// @Router /ping [get].
func (h *PingHandler) Ping(c echo.Context) error {
	return c.JSON(http.StatusOK, PingResponse{
		Status:                "ok",
		ContainerBackend:      ctr.NormalizeBackend(h.runtime.ContainerBackend),
		LocalWorkspaceEnabled: h.cfg.Local.Enabled,
		SnapshotSupported:     h.snapshotSupported(c.Request().Context()),
		Version:               version.Version,
		CommitHash:            version.ShortCommitHash(),
	})
}

func (*PingHandler) PingHead(c echo.Context) error {
	return c.NoContent(http.StatusOK)
}

func (h *PingHandler) snapshotSupported(ctx context.Context) bool {
	switch h.runtime.ContainerBackend {
	case "apple":
		return false
	case ctr.BackendKubernetes, ctr.BackendK8s:
		provider, ok := h.service.(snapshotCapabilityProvider)
		if !ok {
			return false
		}
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return provider.SnapshotSupported(probeCtx)
	default:
		return true
	}
}

func (h *PingHandler) ValidateDirectory(c echo.Context) error {
	var req ValidateDirectoryRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	p := strings.TrimSpace(req.Path)
	if p == "" {
		return c.JSON(http.StatusOK, ValidateDirectoryResponse{Valid: false, Error: "path is empty"})
	}

	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusOK, ValidateDirectoryResponse{Valid: false, Error: "directory does not exist"})
		}
		return c.JSON(http.StatusOK, ValidateDirectoryResponse{Valid: false, Error: err.Error()})
	}

	if !info.IsDir() {
		return c.JSON(http.StatusOK, ValidateDirectoryResponse{Valid: false, Error: "path is not a directory"})
	}

	return c.JSON(http.StatusOK, ValidateDirectoryResponse{Valid: true})
}

func (h *PingHandler) ListDirectory(c echo.Context) error {
	var req ListDirectoryRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	dirPath := strings.TrimSpace(req.Path)
	if dirPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "cannot determine home directory")
		}
		dirPath = home
	}

	dirPath = filepath.Clean(dirPath)

	info, err := os.Stat(dirPath)
	if err != nil {
		return c.JSON(http.StatusOK, ListDirectoryResponse{Path: dirPath, Entries: []ListDirectoryEntry{}})
	}
	if !info.IsDir() {
		dirPath = filepath.Dir(dirPath)
	}

	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return c.JSON(http.StatusOK, ListDirectoryResponse{Path: dirPath, Entries: []ListDirectoryEntry{}})
	}

	var entries []ListDirectoryEntry
	for _, e := range dirEntries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !e.IsDir() {
			continue
		}
		entries = append(entries, ListDirectoryEntry{
			Name:  e.Name(),
			Path:  filepath.Join(dirPath, e.Name()),
			IsDir: true,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	parent := filepath.Dir(dirPath)
	if parent == dirPath {
		parent = ""
	}

	return c.JSON(http.StatusOK, ListDirectoryResponse{
		Path:    dirPath,
		Parent:  parent,
		Entries: entries,
	})
}
