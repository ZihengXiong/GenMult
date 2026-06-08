package agenthub

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	sdk "github.com/memohai/twilight-ai/sdk"

	orch "github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
	"github.com/ZihengXiong/GenMult/internal/agenthub/providers"
	"github.com/ZihengXiong/GenMult/internal/config"
	"github.com/ZihengXiong/GenMult/internal/db"
	postgresstore "github.com/ZihengXiong/GenMult/internal/db/postgres/store"
	sqlitestore "github.com/ZihengXiong/GenMult/internal/db/sqlite/store"
	"github.com/ZihengXiong/GenMult/internal/models"
	"github.com/ZihengXiong/GenMult/internal/workspace"
)

type OrchestratorService struct {
	rooms *Service
	orch  *orch.Service
	log   *slog.Logger

	// projSeq tracks the last orchestrator event seq projected to each room as a
	// chat message, so projectRun can incrementally and idempotently surface new
	// run events. In-memory only (M1); a restart may re-project a run's events.
	projMu  sync.Mutex
	projSeq map[string]int64
}

type StartRunRequest struct {
	Objective        string                 `json:"objective"`
	TriggerMessageID string                 `json:"trigger_message_id,omitempty"`
	CreatedBy        string                 `json:"created_by,omitempty"`
	Agents           []orch.AgentDescriptor `json:"agents,omitempty"`
	Metadata         map[string]any         `json:"metadata,omitempty"`
	AutoDispatch     *bool                  `json:"auto_dispatch,omitempty"`
}

func NewOrchestratorService(
	log *slog.Logger,
	cfg config.Config,
	pgStore *postgresstore.Store,
	sqliteStore *sqlitestore.Store,
	roomService *Service,
	wsManager *workspace.Manager,
) (*OrchestratorService, error) {
	if log == nil {
		log = slog.Default()
	}
	var (
		store orch.Store
		err   error
	)
	switch db.DriverFromConfig(cfg) {
	case db.DriverPostgres:
		if pgStore == nil || pgStore.Pool() == nil {
			return nil, errors.New("postgres orchestrator store not configured")
		}
		store, err = orch.NewPostgresSQLStore(pgStore.Pool())
	case db.DriverSQLite:
		if sqliteStore == nil || sqliteStore.DB() == nil {
			return nil, errors.New("sqlite orchestrator store not configured")
		}
		store, err = orch.NewSQLiteSQLStore(sqliteStore.DB())
	default:
		err = errors.New("unsupported database driver for orchestrator")
	}
	if err != nil {
		return nil, err
	}

	resolver := providers.NewDefaultWorkspaceResolver(wsManager)
	var provCfg providers.ProviderConfigs
	provCfg.FromEnvWithDefaults()

	claudeProvider := providers.NewClaudeCodeProvider(provCfg.ClaudeCode, resolver, store, nil, log)
	codexProvider := providers.NewCodexProvider(provCfg.Codex, resolver, store, nil, log)

	registry := orch.NewProviderRegistry(
		claudeProvider,
		codexProvider,
		orch.NoopProvider{},
	)

	orchestrator := orch.NewService(store, buildPlanner(provCfg.ClaudeCode, log), registry, log, orch.Config{
		MaxParallelPerRun:   3,
		MaxParallelPerAgent: 1,
		DispatchAsync:       false,
	})
	return &OrchestratorService{
		rooms:   roomService,
		orch:    orchestrator,
		log:     log.With(slog.String("service", "agenthub_orchestrator")),
		projSeq: make(map[string]int64),
	}, nil
}

func (s *OrchestratorService) StartRun(ctx context.Context, ownerUserID, roomID string, req StartRunRequest) (orch.RunSnapshot, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return orch.RunSnapshot{}, orch.ErrInvalidInput
	}
	room, err := s.rooms.Get(ctx, ownerUserID, roomID)
	if err != nil {
		return orch.RunSnapshot{}, err
	}
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return orch.RunSnapshot{}, orch.ErrInvalidInput
	}
	agents := req.Agents
	if len(agents) == 0 {
		agents = agentsFromRoom(room)
	}
	autoDispatch := true
	if req.AutoDispatch != nil {
		autoDispatch = *req.AutoDispatch
	}
	snapshot, err := s.orch.StartRun(ctx, orch.StartRunInput{
		RoomID:           roomID,
		TriggerMessageID: strings.TrimSpace(req.TriggerMessageID),
		Objective:        objective,
		CreatedBy:        firstNonEmpty(strings.TrimSpace(req.CreatedBy), ownerUserID),
		Agents:           agents,
		Metadata:         req.Metadata,
		AutoDispatch:     autoDispatch,
	})
	if err != nil {
		return snapshot, err
	}
	s.projectRun(ctx, ownerUserID, snapshot)
	return snapshot, nil
}

func (s *OrchestratorService) GetSnapshot(ctx context.Context, ownerUserID, runID string) (orch.RunSnapshot, error) {
	snapshot, err := s.orch.GetSnapshot(ctx, strings.TrimSpace(runID))
	if err != nil {
		return orch.RunSnapshot{}, err
	}
	if _, err := s.rooms.Get(ctx, ownerUserID, snapshot.Run.RoomID); err != nil {
		return orch.RunSnapshot{}, err
	}
	return snapshot, nil
}

// GetLatestRoomRun returns the most recent run snapshot for a room the caller
// owns, or orch.ErrNotFound (→ 404) if the room has no runs yet.
func (s *OrchestratorService) GetLatestRoomRun(ctx context.Context, ownerUserID, roomID string) (orch.RunSnapshot, error) {
	if _, err := s.rooms.Get(ctx, ownerUserID, strings.TrimSpace(roomID)); err != nil {
		return orch.RunSnapshot{}, err
	}
	return s.orch.LatestSnapshotByRoom(ctx, strings.TrimSpace(roomID))
}

func (s *OrchestratorService) ReconcileRun(ctx context.Context, ownerUserID, runID string) (orch.RunSnapshot, error) {
	snapshot, err := s.GetSnapshot(ctx, ownerUserID, runID)
	if err != nil {
		return orch.RunSnapshot{}, err
	}
	reconciled, err := s.orch.ReconcileRun(ctx, snapshot.Run.ID)
	if err != nil {
		return reconciled, err
	}
	s.projectRun(ctx, ownerUserID, reconciled)
	return reconciled, nil
}

func (s *OrchestratorService) CancelRun(ctx context.Context, ownerUserID, runID string) (orch.RunSnapshot, error) {
	snapshot, err := s.GetSnapshot(ctx, ownerUserID, runID)
	if err != nil {
		return orch.RunSnapshot{}, err
	}
	return s.orch.CancelRun(ctx, snapshot.Run.ID)
}

func (s *OrchestratorService) ListEvents(ctx context.Context, ownerUserID, runID string, afterSeq int64, limit int32) ([]orch.RunEvent, error) {
	snapshot, err := s.GetSnapshot(ctx, ownerUserID, runID)
	if err != nil {
		return nil, err
	}
	return s.orch.ListEvents(ctx, snapshot.Run.ID, afterSeq, limit)
}

func (s *OrchestratorService) ReconcileActiveRuns(ctx context.Context, ownerUserID string) ([]orch.RunSnapshot, error) {
	roomsResp, err := s.rooms.List(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	ownedRooms := make(map[string]struct{}, len(roomsResp.Items))
	for _, room := range roomsResp.Items {
		ownedRooms[room.ID] = struct{}{}
	}
	allRuns, err := s.orch.ListActiveRuns(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]orch.RunSnapshot, 0)
	for _, run := range allRuns {
		if _, ok := ownedRooms[run.RoomID]; !ok {
			continue
		}
		snapshot, err := s.orch.ReconcileRun(ctx, run.ID)
		if err != nil {
			return out, err
		}
		s.projectRun(ctx, ownerUserID, snapshot)
		out = append(out, snapshot)
	}
	return out, nil
}

// buildPlanner returns an LLM-backed planner (with rule-planner fallback) when
// Anthropic credentials are available, otherwise the deterministic rule planner.
// It logs which path is active so "the orchestrator is intelligent" is verifiable.
func buildPlanner(cfg providers.ClaudeCodeConfig, log *slog.Logger) orch.Planner {
	if log == nil {
		log = slog.Default()
	}
	rule := orch.NewRulePlanner()
	model := buildPlannerModel(cfg)
	if model == nil {
		log.Info("agenthub planner: using rule planner (no Anthropic credentials configured for LLM planning)")
		return rule
	}
	log.Info("agenthub planner: using LLM planner with rule-planner fallback")
	return newLLMPlanner(model, rule, log)
}

func buildPlannerModel(cfg providers.ClaudeCodeConfig) *sdk.Model {
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" {
		key = strings.TrimSpace(cfg.AuthToken)
	}
	if key == "" {
		return nil
	}
	modelID := strings.TrimSpace(cfg.Model)
	if modelID == "" {
		modelID = "claude-3-5-sonnet-latest"
	}
	return models.NewSDKChatModel(models.SDKModelConfig{
		ClientType: string(models.ClientTypeAnthropicMessages),
		APIKey:     key,
		BaseURL:    strings.TrimSpace(cfg.BaseURL),
		ModelID:    modelID,
	})
}

func agentsFromRoom(room Room) []orch.AgentDescriptor {
	if len(room.AgentIDs) == 0 {
		return []orch.AgentDescriptor{{ID: "orchestrator", ProviderName: "noop", Name: "Orchestrator", Capabilities: []string{"plan", "code", "test", "review"}}}
	}
	out := make([]orch.AgentDescriptor, 0, len(room.AgentIDs))
	for _, id := range room.AgentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		provider, name := friendlyAgentName(id)
		out = append(out, orch.AgentDescriptor{
			ID:           id,
			ProviderName: provider,
			Name:         name,
			Capabilities: []string{"code", "review"},
			Metadata:     map[string]any{"source": "room", "index": strconv.Itoa(len(out))},
		})
	}
	if len(out) == 0 {
		return []orch.AgentDescriptor{{ID: "orchestrator", ProviderName: "noop", Name: "Orchestrator", Capabilities: []string{"plan", "code", "test", "review"}}}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
