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
	"github.com/ZihengXiong/GenMult/internal/bots"
	"github.com/ZihengXiong/GenMult/internal/config"
	"github.com/ZihengXiong/GenMult/internal/db"
	postgresstore "github.com/ZihengXiong/GenMult/internal/db/postgres/store"
	sqlitestore "github.com/ZihengXiong/GenMult/internal/db/sqlite/store"
	dbstore "github.com/ZihengXiong/GenMult/internal/db/store"
	"github.com/ZihengXiong/GenMult/internal/models"
	globalproviders "github.com/ZihengXiong/GenMult/internal/providers"
	"github.com/ZihengXiong/GenMult/internal/workspace"
)

type OrchestratorService struct {
	rooms *Service
	orch  *orch.Service
	bots  *bots.Service
	log   *slog.Logger

	// projSeq tracks the last orchestrator event seq projected to each room as a
	// chat message, so projectRun can incrementally and idempotently surface new
	// run events. Idempotency is correct within a process lifetime.
	//
	// DURABILITY CONTRACT: this map is in-memory only, so after a restart it
	// starts empty and projectRun(after=0) would re-post a run's full event
	// history. Today that path is unreachable — ReconcileActiveRuns is only
	// exposed via POST /runs/reconcile-active and is never called automatically
	// (no client hits it, no boot-time self-heal). Before wiring any automatic
	// reconcile / boot-time re-projection, make projection idempotent at the
	// insert layer instead of relying on this map: each projected message already
	// carries {run_id, event_seq} in its metadata (see roomMessageForEvent), so a
	// unique constraint on (run_id, event_seq) + INSERT … ON CONFLICT DO NOTHING
	// closes the restart window fully (a persisted high-water mark would still
	// leave a crash gap between the message insert and the seq write).
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
	memohRunner providers.MemohRunner,
	botService *bots.Service,
	queries dbstore.Queries,
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

	// Reuse the same DB credential source as single-chat (the 通用设置
	// DeepSeek/Anthropic-compatible provider) so claudecode tasks don't require
	// ANTHROPIC_API_KEY in the environment.
	claudeCredResolver := func(ctx context.Context) (apiKey, baseURL string) {
		if queries == nil {
			return "", ""
		}
		creds, err := globalproviders.ResolveCredentialsForFramework(ctx, queries, "claudecode")
		if err != nil {
			return "", ""
		}
		return creds.APIKey, creds.BaseURL
	}

	claudeProvider := providers.NewClaudeCodeProvider(provCfg.ClaudeCode, resolver, store, nil, claudeCredResolver, log)
	codexProvider := providers.NewCodexProvider(provCfg.Codex, resolver, store, nil, log)
	memohProvider := providers.NewMemohProvider(memohRunner, store, log)

	registry := orch.NewProviderRegistry(
		claudeProvider,
		codexProvider,
		memohProvider,
		orch.NoopProvider{},
	)

	orchestrator := orch.NewService(store, buildPlanner(provCfg.ClaudeCode, log), registry, log, orch.Config{
		MaxParallelPerRun:   3,
		MaxParallelPerAgent: 1,
		DispatchAsync:       true,
	})
	return &OrchestratorService{
		rooms:   roomService,
		orch:    orchestrator,
		bots:    botService,
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
		agents = s.agentsFromRoom(ctx, room)
	}
	autoDispatch := true
	if req.AutoDispatch != nil {
		autoDispatch = *req.AutoDispatch
	}
	// Carry the room's recent conversation as run metadata so the planner and
	// the executing agents have multi-turn context ("上下文连续/多轮迭代修改").
	metadata := make(map[string]any, len(req.Metadata)+1)
	for k, v := range req.Metadata {
		metadata[k] = v
	}
	if _, exists := metadata["room_history"]; !exists {
		// Pinned messages are surfaced as long-term context ahead of the rolling
		// recent-history window, so user-curated key facts persist across runs even
		// once they scroll out of the recent window ("手动 pin 关键消息作为长期上下文").
		pinned := s.pinnedRoomContext(ctx, ownerUserID, roomID, room)
		hist := s.recentRoomHistory(ctx, ownerUserID, roomID, 20)
		if combined := joinContextBlocks(pinned, hist); combined != "" {
			metadata["room_history"] = combined
		}
	}
	snapshot, err := s.orch.StartRun(ctx, orch.StartRunInput{
		RoomID:           roomID,
		TriggerMessageID: strings.TrimSpace(req.TriggerMessageID),
		Objective:        objective,
		CreatedBy:        firstNonEmpty(strings.TrimSpace(req.CreatedBy), ownerUserID),
		Agents:           agents,
		Metadata:         metadata,
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
		// NOTE: across a process restart projSeq is empty, so this re-projects the
		// run's whole event history as duplicate room messages. Safe only while
		// this entry point isn't called automatically — see the projSeq durability
		// contract before wiring boot-time / periodic reconcile.
		s.projectRun(ctx, ownerUserID, snapshot)
		out = append(out, snapshot)
	}
	return out, nil
}

// buildPlanner returns an LLM-backed planner (with rule-planner fallback) when
// Anthropic credentials are available, otherwise the deterministic rule planner.
// It logs which path is active so "the orchestrator is intelligent" is verifiable.
// recentRoomHistory renders the room's most recent messages as a plain-text
// transcript (oldest→newest) for use as orchestrator run context. Best-effort:
// returns "" on any error.
func (s *OrchestratorService) recentRoomHistory(ctx context.Context, ownerUserID, roomID string, limit int32) string {
	resp, err := s.rooms.ListMessages(ctx, ownerUserID, roomID, limit)
	if err != nil || len(resp.Items) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range resp.Items {
		body := strings.TrimSpace(m.Body)
		if body == "" {
			continue
		}
		name := strings.TrimSpace(m.SenderName)
		if name == "" {
			name = strings.TrimSpace(m.SenderType)
		}
		b.WriteString(name)
		b.WriteString("：")
		b.WriteString(body)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// pinnedRoomContext renders the room's user-pinned messages (room.metadata
// .pinned_message_ids) as a labelled long-term-context block. Best-effort:
// returns "" when there are no pins or on any lookup error.
func (s *OrchestratorService) pinnedRoomContext(ctx context.Context, ownerUserID, roomID string, room Room) string {
	ids := pinnedMessageIDs(room.Metadata)
	if len(ids) == 0 {
		return ""
	}
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	// Pull a wide window so pins that scrolled out of the recent window are still
	// found; pins are few so the linear scan is cheap.
	resp, err := s.rooms.ListMessages(ctx, ownerUserID, roomID, 500)
	if err != nil || len(resp.Items) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range resp.Items {
		if _, ok := want[m.ID]; !ok {
			continue
		}
		body := strings.TrimSpace(m.Body)
		if body == "" {
			continue
		}
		name := strings.TrimSpace(m.SenderName)
		if name == "" {
			name = strings.TrimSpace(m.SenderType)
		}
		b.WriteString("📌 ")
		b.WriteString(name)
		b.WriteString("：")
		b.WriteString(body)
		b.WriteString("\n")
	}
	pinned := strings.TrimSpace(b.String())
	if pinned == "" {
		return ""
	}
	return "用户置顶的关键信息（长期上下文，务必优先考虑）：\n" + pinned
}

// pinnedMessageIDs extracts the pinned_message_ids string slice from room
// metadata, tolerating the []any shape JSON unmarshals into.
func pinnedMessageIDs(metadata map[string]any) []string {
	raw, ok := metadata["pinned_message_ids"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// joinContextBlocks joins non-empty context blocks with a blank line separator.
func joinContextBlocks(blocks ...string) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if s := strings.TrimSpace(b); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
}

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

func (s *OrchestratorService) agentsFromRoom(ctx context.Context, room Room) []orch.AgentDescriptor {
	defaultCaps := []string{"plan", "code", "test", "review"}
	if len(room.AgentIDs) == 0 {
		return []orch.AgentDescriptor{{ID: "orchestrator", ProviderName: "noop", Name: "Orchestrator", Capabilities: defaultCaps}}
	}
	out := make([]orch.AgentDescriptor, 0, len(room.AgentIDs))
	for _, id := range room.AgentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		provider, name := s.resolveAgentProvider(ctx, id)
		caps := defaultCaps
		switch provider {
		case "claudecode", "codex":
			caps = []string{"plan", "code", "test", "review", "edit", "exec"}
		case "memoh":
			caps = []string{"plan", "analysis", "review", "chat"}
		}
		out = append(out, orch.AgentDescriptor{
			ID:           id,
			ProviderName: provider,
			Name:         name,
			Capabilities: caps,
			Metadata:     map[string]any{"source": "room", "index": strconv.Itoa(len(out))},
		})
	}
	if len(out) == 0 {
		return []orch.AgentDescriptor{{ID: "orchestrator", ProviderName: "noop", Name: "Orchestrator", Capabilities: defaultCaps}}
	}
	return out
}

// resolveAgentProvider maps a room agent id to its orchestrator provider name
// and display name. A "bot:UUID" id is resolved against the bot store by the
// bot's framework (claudecode/codex/memoh) so backend-derived runs (those that
// don't carry an explicit Agents list, e.g. reconcile) dispatch to the real
// provider instead of falling through to noop. Non-bot ids and lookup failures
// fall back to the id-heuristic friendlyAgentName.
func (s *OrchestratorService) resolveAgentProvider(ctx context.Context, agentID string) (provider string, name string) {
	id := strings.TrimSpace(agentID)
	if strings.HasPrefix(id, "bot:") && s.bots != nil {
		botID := strings.TrimPrefix(id, "bot:")
		if bot, err := s.bots.Get(ctx, botID); err == nil {
			display := strings.TrimSpace(bot.DisplayName)
			if display == "" {
				display = botID
			}
			switch strings.TrimSpace(bot.Framework) {
			case bots.FrameworkClaudeCode:
				return "claudecode", display
			case bots.FrameworkCodex:
				return "codex", display
			case bots.FrameworkMemoh:
				return "memoh", display
			}
		}
	}
	return friendlyAgentName(id)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
