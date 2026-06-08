package handlers

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

type applyEditRequest struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

// ApplyEdit applies a text edit to a file in the bot's workspace container.
// @Summary Apply edit to workspace file
// @Tags artifacts
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param body body applyEditRequest true "Edit request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /bots/{bot_id}/apply-edit [post].
func (h *ContainerdHandler) ApplyEdit(c echo.Context) error {
	botID := c.Param("bot_id")
	if strings.TrimSpace(botID) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bot_id is required"})
	}

	var req applyEditRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Path) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "path is required"})
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to connect to workspace"})
	}

	existing, err := client.ReadFile(ctx, req.Path, 0, 0)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read file"})
	}

	content := ""
	if existing != nil {
		content = existing.Content
	}
	if req.OldText != "" {
		if !strings.Contains(content, req.OldText) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "old_text not found in file"})
		}
		content = strings.Replace(content, req.OldText, req.NewText, 1)
	} else {
		content = req.NewText
	}

	if err := client.WriteFile(ctx, req.Path, []byte(content)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write file"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "applied"})
}

type applyWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ApplyWrite writes content to a file in the bot's workspace container.
// @Summary Write file to workspace
// @Tags artifacts
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param body body applyWriteRequest true "Write request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /bots/{bot_id}/apply-write [post].
func (h *ContainerdHandler) ApplyWrite(c echo.Context) error {
	botID := c.Param("bot_id")
	if strings.TrimSpace(botID) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bot_id is required"})
	}

	var req applyWriteRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Path) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "path is required"})
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to connect to workspace"})
	}

	if err := client.WriteFile(ctx, req.Path, []byte(req.Content)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write file"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "applied"})
}
