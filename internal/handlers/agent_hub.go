package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/ZihengXiong/GenMult/internal/agenthub"
	"github.com/ZihengXiong/GenMult/internal/auth"
)

type AgentHubHandler struct {
	service *agenthub.Service
	logger  *slog.Logger
}

func NewAgentHubHandler(log *slog.Logger, service *agenthub.Service) *AgentHubHandler {
	if log == nil {
		log = slog.Default()
	}
	return &AgentHubHandler{
		service: service,
		logger:  log.With(slog.String("handler", "agent_hub")),
	}
}

func (h *AgentHubHandler) Register(e *echo.Echo) {
	group := e.Group("/agent-hub")
	group.GET("/rooms", h.ListRooms)
	group.POST("/rooms", h.CreateRoom)
	group.GET("/rooms/:room_id", h.GetRoom)
	group.PUT("/rooms/:room_id", h.UpdateRoom)
	group.DELETE("/rooms/:room_id", h.DeleteRoom)
	group.POST("/rooms/:room_id/agents", h.AddAgent)
	group.DELETE("/rooms/:room_id/agents/:agent_id", h.RemoveAgent)
	group.GET("/rooms/:room_id/messages", h.ListMessages)
	group.POST("/rooms/:room_id/messages", h.CreateMessage)
	group.DELETE("/rooms/:room_id/messages/:message_id", h.DeleteMessage)
}

// ListRooms godoc
// @Summary List AgentHub rooms
// @Description List AgentHub group rooms owned by the current user.
// @Tags agent-hub
// @Produce json
// @Success 200 {object} agenthub.ListRoomsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms [get].
func (h *AgentHubHandler) ListRooms(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	resp, err := h.service.List(c.Request().Context(), ownerID)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, resp)
}

// CreateRoom godoc
// @Summary Create AgentHub room
// @Description Create an AgentHub group room and optional initial agent membership.
// @Tags agent-hub
// @Accept json
// @Produce json
// @Param request body agenthub.UpsertRoomRequest true "Room payload"
// @Success 201 {object} agenthub.Room
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms [post].
func (h *AgentHubHandler) CreateRoom(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	var req agenthub.UpsertRoomRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	room, err := h.service.Create(c.Request().Context(), ownerID, req)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusCreated, room)
}

// GetRoom godoc
// @Summary Get AgentHub room
// @Description Get an AgentHub group room by id.
// @Tags agent-hub
// @Produce json
// @Param room_id path string true "Room ID"
// @Success 200 {object} agenthub.Room
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms/{room_id} [get].
func (h *AgentHubHandler) GetRoom(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	room, err := h.service.Get(c.Request().Context(), ownerID, c.Param("room_id"))
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, room)
}

// UpdateRoom godoc
// @Summary Update AgentHub room
// @Description Update an AgentHub group room and its agent membership.
// @Tags agent-hub
// @Accept json
// @Produce json
// @Param room_id path string true "Room ID"
// @Param request body agenthub.UpsertRoomRequest true "Room payload"
// @Success 200 {object} agenthub.Room
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms/{room_id} [put].
func (h *AgentHubHandler) UpdateRoom(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	var req agenthub.UpsertRoomRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	room, err := h.service.Update(c.Request().Context(), ownerID, c.Param("room_id"), req)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, room)
}

// DeleteRoom godoc
// @Summary Delete AgentHub room
// @Description Delete an AgentHub group room.
// @Tags agent-hub
// @Param room_id path string true "Room ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms/{room_id} [delete].
func (h *AgentHubHandler) DeleteRoom(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	if err := h.service.Delete(c.Request().Context(), ownerID, c.Param("room_id")); err != nil {
		return h.httpError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// AddAgent godoc
// @Summary Add AgentHub room agent
// @Description Add an agent member to an AgentHub group room.
// @Tags agent-hub
// @Accept json
// @Produce json
// @Param room_id path string true "Room ID"
// @Param request body agenthub.AddAgentRequest true "Agent payload"
// @Success 200 {object} agenthub.Room
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms/{room_id}/agents [post].
func (h *AgentHubHandler) AddAgent(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	var req agenthub.AddAgentRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	room, err := h.service.AddAgent(c.Request().Context(), ownerID, c.Param("room_id"), req.AgentID)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, room)
}

// RemoveAgent godoc
// @Summary Remove AgentHub room agent
// @Description Remove an agent member from an AgentHub group room.
// @Tags agent-hub
// @Produce json
// @Param room_id path string true "Room ID"
// @Param agent_id path string true "Agent ID"
// @Success 200 {object} agenthub.Room
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms/{room_id}/agents/{agent_id} [delete].
func (h *AgentHubHandler) RemoveAgent(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	agentID, err := url.PathUnescape(c.Param("agent_id"))
	if err != nil {
		agentID = c.Param("agent_id")
	}
	room, err := h.service.RemoveAgent(c.Request().Context(), ownerID, c.Param("room_id"), agentID)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, room)
}

// ListMessages godoc
// @Summary List AgentHub room messages
// @Description List persisted timeline messages for an AgentHub group room.
// @Tags agent-hub
// @Produce json
// @Param room_id path string true "Room ID"
// @Param limit query int false "Maximum messages"
// @Success 200 {object} agenthub.ListMessagesResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms/{room_id}/messages [get].
func (h *AgentHubHandler) ListMessages(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	limit := int32(200)
	if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid limit")
		}
		limit = int32(parsed) //nolint:gosec // parsed from string, acceptable to wrap
	}
	resp, err := h.service.ListMessages(c.Request().Context(), ownerID, c.Param("room_id"), limit)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, resp)
}

// CreateMessage godoc
// @Summary Create AgentHub room message
// @Description Append a message to an AgentHub group room timeline.
// @Tags agent-hub
// @Accept json
// @Produce json
// @Param room_id path string true "Room ID"
// @Param request body agenthub.CreateMessageRequest true "Message payload"
// @Success 201 {object} agenthub.Message
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms/{room_id}/messages [post].
func (h *AgentHubHandler) CreateMessage(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	var req agenthub.CreateMessageRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	msg, err := h.service.CreateMessage(c.Request().Context(), ownerID, c.Param("room_id"), req)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusCreated, msg)
}

// DeleteMessage godoc
// @Summary Delete AgentHub room message
// @Description Delete a single message from an AgentHub group room timeline.
// @Tags agent-hub
// @Param room_id path string true "Room ID"
// @Param message_id path string true "Message ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms/{room_id}/messages/{message_id} [delete].
func (h *AgentHubHandler) DeleteMessage(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	if err := h.service.DeleteMessage(c.Request().Context(), ownerID, c.Param("room_id"), c.Param("message_id")); err != nil {
		return h.httpError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *AgentHubHandler) httpError(err error) error {
	switch {
	case errors.Is(err, agenthub.ErrInvalidOwner), errors.Is(err, agenthub.ErrInvalidRoomID), errors.Is(err, agenthub.ErrInvalidMessageID):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, agenthub.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case strings.Contains(err.Error(), "required"):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		h.logger.Error("agent hub request failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}
