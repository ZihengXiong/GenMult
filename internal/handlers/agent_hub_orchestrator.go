package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/ZihengXiong/GenMult/internal/agenthub"
	orch "github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
	"github.com/ZihengXiong/GenMult/internal/auth"
)

type AgentHubOrchestratorHandler struct {
	service *agenthub.OrchestratorService
	logger  *slog.Logger
}

func NewAgentHubOrchestratorHandler(log *slog.Logger, service *agenthub.OrchestratorService) *AgentHubOrchestratorHandler {
	if log == nil {
		log = slog.Default()
	}
	return &AgentHubOrchestratorHandler{
		service: service,
		logger:  log.With(slog.String("handler", "agent_hub_orchestrator")),
	}
}

func (h *AgentHubOrchestratorHandler) Register(e *echo.Echo) {
	group := e.Group("/agent-hub")
	group.POST("/rooms/:room_id/runs", h.StartRun)
	group.GET("/rooms/:room_id/runs/latest", h.GetLatestRoomRun)
	group.GET("/runs/:run_id", h.GetRun)
	group.POST("/runs/:run_id/reconcile", h.ReconcileRun)
	group.POST("/runs/:run_id/confirm", h.ConfirmRun)
	group.POST("/runs/:run_id/cancel", h.CancelRun)
	group.GET("/runs/:run_id/events", h.ListRunEvents)
	group.POST("/runs/reconcile-active", h.ReconcileActiveRuns)
}

// StartRun godoc
// @Summary Start AgentHub orchestration run
// @Description Plan objective into tasks and optionally dispatch immediately.
// @Tags agent-hub
// @Accept json
// @Produce json
// @Param room_id path string true "Room ID"
// @Param request body agenthub.StartRunRequest true "Start run payload"
// @Success 201 {object} orchestrator.RunSnapshot
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms/{room_id}/runs [post].
func (h *AgentHubOrchestratorHandler) StartRun(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	var req agenthub.StartRunRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	snapshot, err := h.service.StartRun(c.Request().Context(), ownerID, c.Param("room_id"), req)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusCreated, snapshot)
}

// GetLatestRoomRun godoc
// @Summary Get the latest AgentHub run for a room
// @Description Return the most recent run snapshot for a room, or 404 if none.
// @Tags agent-hub
// @Produce json
// @Param room_id path string true "Room ID"
// @Success 200 {object} orchestrator.RunSnapshot
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/rooms/{room_id}/runs/latest [get].
func (h *AgentHubOrchestratorHandler) GetLatestRoomRun(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	snapshot, err := h.service.GetLatestRoomRun(c.Request().Context(), ownerID, c.Param("room_id"))
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, snapshot)
}

// GetRun godoc
// @Summary Get AgentHub orchestration run snapshot
// @Description Return run state, tasks, dependencies and attempts.
// @Tags agent-hub
// @Produce json
// @Param run_id path string true "Run ID"
// @Success 200 {object} orchestrator.RunSnapshot
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/runs/{run_id} [get].
func (h *AgentHubOrchestratorHandler) GetRun(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	snapshot, err := h.service.GetSnapshot(c.Request().Context(), ownerID, c.Param("run_id"))
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, snapshot)
}

// ReconcileRun godoc
// @Summary Reconcile AgentHub run
// @Description Recompute task readiness, dispatch runnable tasks, and advance run status.
// @Tags agent-hub
// @Produce json
// @Param run_id path string true "Run ID"
// @Success 200 {object} orchestrator.RunSnapshot
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/runs/{run_id}/reconcile [post].
func (h *AgentHubOrchestratorHandler) ReconcileRun(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	snapshot, err := h.service.ReconcileRun(c.Request().Context(), ownerID, c.Param("run_id"))
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, snapshot)
}

// ConfirmRun godoc
// @Summary Confirm a gated AgentHub run
// @Description Clear the plan-confirmation hold and dispatch the planned tasks.
// @Tags agent-hub
// @Produce json
// @Param run_id path string true "Run ID"
// @Success 200 {object} orchestrator.RunSnapshot
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/runs/{run_id}/confirm [post].
func (h *AgentHubOrchestratorHandler) ConfirmRun(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	snapshot, err := h.service.ConfirmRun(c.Request().Context(), ownerID, c.Param("run_id"))
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, snapshot)
}

// CancelRun godoc
// @Summary Cancel AgentHub run
// @Description Cancel a run and mark non-terminal tasks cancelled.
// @Tags agent-hub
// @Produce json
// @Param run_id path string true "Run ID"
// @Success 200 {object} orchestrator.RunSnapshot
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/runs/{run_id}/cancel [post].
func (h *AgentHubOrchestratorHandler) CancelRun(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	snapshot, err := h.service.CancelRun(c.Request().Context(), ownerID, c.Param("run_id"))
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, snapshot)
}

// ListRunEvents godoc
// @Summary List AgentHub run events
// @Description Return append-only orchestrator events for timeline projection.
// @Tags agent-hub
// @Produce json
// @Param run_id path string true "Run ID"
// @Param after_seq query int false "Return events with seq > after_seq"
// @Param limit query int false "Maximum events"
// @Success 200 {array} orchestrator.RunEvent
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/runs/{run_id}/events [get].
func (h *AgentHubOrchestratorHandler) ListRunEvents(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	afterSeq := int64(0)
	if raw := strings.TrimSpace(c.QueryParam("after_seq")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid after_seq")
		}
		afterSeq = parsed
	}
	limit := int32(200)
	if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid limit")
		}
		limit = int32(parsed) //nolint:gosec // parsed from string, acceptable to wrap
	}
	events, err := h.service.ListEvents(c.Request().Context(), ownerID, c.Param("run_id"), afterSeq, limit)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, events)
}

// ReconcileActiveRuns godoc
// @Summary Reconcile all active AgentHub runs for current owner
// @Description Trigger reconciliation for planning/dispatching/collecting runs.
// @Tags agent-hub
// @Produce json
// @Success 200 {array} orchestrator.RunSnapshot
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent-hub/runs/reconcile-active [post].
func (h *AgentHubOrchestratorHandler) ReconcileActiveRuns(c echo.Context) error {
	ownerID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	snapshots, err := h.service.ReconcileActiveRuns(c.Request().Context(), ownerID)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, snapshots)
}

func (h *AgentHubOrchestratorHandler) httpError(err error) error {
	switch {
	case errors.Is(err, agenthub.ErrInvalidOwner), errors.Is(err, agenthub.ErrInvalidRoomID):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, agenthub.ErrNotFound), errors.Is(err, orch.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, orch.ErrInvalidInput), errors.Is(err, orch.ErrInvalidTransition), errors.Is(err, orch.ErrDuplicateClientKey):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		h.logger.Error("agent hub orchestrator request failed", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}
