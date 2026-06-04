package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/ZihengXiong/GenMult/internal/accounts"
	"github.com/ZihengXiong/GenMult/internal/bots"
	"github.com/ZihengXiong/GenMult/internal/mcp"
	skillset "github.com/ZihengXiong/GenMult/internal/skills"
	"github.com/ZihengXiong/GenMult/internal/supermarket"
	"github.com/ZihengXiong/GenMult/internal/workspace/bridge"
)

type SupermarketHandler struct {
	registry       *supermarket.Registry
	mcpService     *mcp.ConnectionService
	containers     bridge.Provider
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

func NewSupermarketHandler(
	log *slog.Logger,
	registry *supermarket.Registry,
	mcpService *mcp.ConnectionService,
	containers bridge.Provider,
	botService *bots.Service,
	accountService *accounts.Service,
) *SupermarketHandler {
	return &SupermarketHandler{
		registry:       registry,
		mcpService:     mcpService,
		containers:     containers,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "supermarket")),
	}
}

func (h *SupermarketHandler) Register(e *echo.Echo) {
	g := e.Group("/supermarket")
	g.GET("/mcps", h.ListMcps)
	g.GET("/mcps/:id", h.GetMcp)
	g.GET("/skills", h.ListSkills)
	g.GET("/skills/:id", h.GetSkill)
	g.GET("/tags", h.ListTags)

	ig := e.Group("/bots/:bot_id/supermarket")
	ig.POST("/install-mcp", h.InstallMcp)
	ig.POST("/install-skill", h.InstallSkill)
}

func (h *SupermarketHandler) requireBotAccess(c echo.Context) (string, error) {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return "", err
	}
	botID := c.Param("bot_id")
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return "", err
	}
	return botID, nil
}

// ListMcps godoc
// @Summary List MCPs from supermarket
// @Tags supermarket
// @Param q query string false "Search query"
// @Param tag query string false "Filter by tag"
// @Param transport query string false "Filter by transport type"
// @Param page query int false "Page number"
// @Param limit query int false "Items per page"
// @Success 200 {object} SupermarketMcpListResponse
// @Router /supermarket/mcps [get].
func (h *SupermarketHandler) ListMcps(c echo.Context) error {
	q := c.QueryParam("q")
	tag := c.QueryParam("tag")
	transport := c.QueryParam("transport")
	page := queryInt(c, "page", 1)
	limit := queryInt(c, "limit", 20)

	result := h.registry.ListMcps(q, tag, transport, page, limit)
	return c.JSON(http.StatusOK, result)
}

// GetMcp godoc
// @Summary Get MCP detail from supermarket
// @Tags supermarket
// @Param id path string true "MCP ID"
// @Success 200 {object} SupermarketMcpEntry
// @Failure 404 {object} ErrorResponse
// @Router /supermarket/mcps/{id} [get].
func (h *SupermarketHandler) GetMcp(c echo.Context) error {
	id := c.Param("id")
	entry, ok := h.registry.GetMcp(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("MCP %q not found", id))
	}
	return c.JSON(http.StatusOK, entry)
}

// ListSkills godoc
// @Summary List skills from supermarket
// @Tags supermarket
// @Param q query string false "Search query"
// @Param tag query string false "Filter by tag"
// @Param page query int false "Page number"
// @Param limit query int false "Items per page"
// @Success 200 {object} SupermarketSkillListResponse
// @Router /supermarket/skills [get].
func (h *SupermarketHandler) ListSkills(c echo.Context) error {
	q := c.QueryParam("q")
	tag := c.QueryParam("tag")
	page := queryInt(c, "page", 1)
	limit := queryInt(c, "limit", 20)

	result := h.registry.ListSkills(q, tag, page, limit)
	return c.JSON(http.StatusOK, result)
}

// GetSkill godoc
// @Summary Get skill detail from supermarket
// @Tags supermarket
// @Param id path string true "Skill ID"
// @Success 200 {object} SupermarketSkillEntry
// @Failure 404 {object} ErrorResponse
// @Router /supermarket/skills/{id} [get].
func (h *SupermarketHandler) GetSkill(c echo.Context) error {
	id := c.Param("id")
	entry, ok := h.registry.GetSkill(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("skill %q not found", id))
	}
	return c.JSON(http.StatusOK, entry)
}

// ListTags godoc
// @Summary List all tags from supermarket
// @Tags supermarket
// @Success 200 {object} SupermarketTagsResponse
// @Router /supermarket/tags [get].
func (h *SupermarketHandler) ListTags(c echo.Context) error {
	result := h.registry.ListTags()
	return c.JSON(http.StatusOK, result)
}

// --- Install endpoints ---

// InstallMcpRequest is the request body for installing an MCP from supermarket.
type InstallMcpRequest struct {
	McpID string            `json:"mcp_id"`
	Env   map[string]string `json:"env,omitempty"`
}

// InstallSkillRequest is the request body for installing a skill from supermarket.
type InstallSkillRequest struct {
	SkillID string `json:"skill_id"`
}

// InstallMcp godoc
// @Summary Install MCP from supermarket to bot
// @Tags supermarket
// @Param bot_id path string true "Bot ID"
// @Param payload body InstallMcpRequest true "Install MCP request"
// @Success 200 {object} mcp.Connection
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/supermarket/install-mcp [post].
func (h *SupermarketHandler) InstallMcp(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req InstallMcpRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.McpID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "mcp_id is required")
	}

	entry, ok := h.registry.GetMcp(req.McpID)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("MCP %q not found", req.McpID))
	}

	upsert := mcpEntryToUpsert(entry, req.Env)
	conn, err := h.mcpService.Create(c.Request().Context(), botID, upsert)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, conn)
}

// InstallSkill godoc
// @Summary Install skill from supermarket to bot container
// @Tags supermarket
// @Param bot_id path string true "Bot ID"
// @Param payload body InstallSkillRequest true "Install skill request"
// @Success 200 {object} map[string]bool
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/supermarket/install-skill [post].
func (h *SupermarketHandler) InstallSkill(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req InstallSkillRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	skillID := strings.TrimSpace(req.SkillID)
	if skillID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "skill_id is required")
	}
	if strings.Contains(skillID, "..") || strings.Contains(skillID, "/") {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid skill_id")
	}

	files, err := h.registry.GetSkillFiles(skillID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("skill %q not found", skillID))
	}

	ctx := c.Request().Context()
	client, err := h.containers.MCPClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("container not reachable: %v", err))
	}

	skillDir := path.Join(skillset.ManagedDir(), skillID)
	if err := client.Mkdir(ctx, skillDir); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("mkdir failed: %v", err))
	}

	filesWritten := 0
	for name, content := range files {
		filePath := path.Join(skillDir, name)
		if err := client.WriteFile(ctx, filePath, content); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("write file %s failed: %v", name, err))
		}
		filesWritten++
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true, "files_written": filesWritten})
}

// --- Supermarket upstream types (for swagger) ---

type SupermarketAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type SupermarketConfigVar struct {
	Key          string `json:"key"`
	Description  string `json:"description"`
	DefaultValue string `json:"defaultValue,omitempty"`
}

type SupermarketMcpEntry struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Author      SupermarketAuthor      `json:"author"`
	Transport   string                 `json:"transport"`
	Icon        string                 `json:"icon,omitempty"`
	Homepage    string                 `json:"homepage,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	URL         string                 `json:"url,omitempty"`
	Command     string                 `json:"command,omitempty"`
	Args        []string               `json:"args,omitempty"`
	Headers     []SupermarketConfigVar `json:"headers,omitempty"`
	Env         []SupermarketConfigVar `json:"env,omitempty"`
}

type SupermarketMcpListResponse struct {
	Total int                   `json:"total"`
	Page  int                   `json:"page"`
	Limit int                   `json:"limit"`
	Data  []SupermarketMcpEntry `json:"data"`
}

type SupermarketSkillMetadata struct {
	Author   SupermarketAuthor `json:"author"`
	Tags     []string          `json:"tags,omitempty"`
	Homepage string            `json:"homepage,omitempty"`
}

type SupermarketSkillEntry struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Metadata    SupermarketSkillMetadata `json:"metadata"`
	Content     string                   `json:"content"`
	Files       []string                 `json:"files"`
}

type SupermarketSkillListResponse struct {
	Total int                     `json:"total"`
	Page  int                     `json:"page"`
	Limit int                     `json:"limit"`
	Data  []SupermarketSkillEntry `json:"data"`
}

type SupermarketTagsResponse struct {
	Tags []string `json:"tags"`
}

// --- Internal helpers ---

func mcpEntryToUpsert(entry supermarket.McpEntry, envOverrides map[string]string) mcp.UpsertRequest {
	headers := make(map[string]string, len(entry.Headers))
	for _, hdr := range entry.Headers {
		headers[hdr.Key] = hdr.DefaultValue
	}

	env := make(map[string]string, len(entry.Env))
	for _, e := range entry.Env {
		if override, ok := envOverrides[e.Key]; ok {
			env[e.Key] = override
		} else {
			env[e.Key] = e.DefaultValue
		}
	}

	return mcp.UpsertRequest{
		Name:      entry.Name,
		Command:   entry.Command,
		Args:      entry.Args,
		URL:       entry.URL,
		Headers:   headers,
		Env:       env,
		Transport: entry.Transport,
	}
}

func queryInt(c echo.Context, key string, defaultVal int) int {
	val := c.QueryParam(key)
	if val == "" {
		return defaultVal
	}
	var n int
	if _, err := fmt.Sscanf(val, "%d", &n); err != nil || n <= 0 {
		return defaultVal
	}
	return n
}
