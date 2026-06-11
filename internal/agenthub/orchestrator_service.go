package agenthub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

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
	"github.com/ZihengXiong/GenMult/internal/settings"
	"github.com/ZihengXiong/GenMult/internal/workspace"
)

type OrchestratorService struct {
	rooms *Service
	orch  *orch.Service
	bots  *bots.Service
	log   *slog.Logger

	// projSeq tracks the last orchestrator event seq projected to each room as a
	// chat message, so projectRun can incrementally surface new run events
	// without re-listing a run's full history on every reconcile.
	//
	// DURABILITY CONTRACT: this map is a per-process fast path, not the
	// idempotency mechanism. Correctness across restarts lives at the insert
	// layer: every projected message carries {run_id, event_seq} in its metadata
	// (see roomMessageForEvent), a partial unique index covers those keys, and
	// the insert runs ON CONFLICT DO NOTHING — a re-projection after the map
	// resets surfaces as ErrDuplicateMessage and is skipped. That is what makes
	// automatic reconcile (RunBackgroundReconciler, POST /runs/reconcile-active)
	// safe to call at any time.
	projMu  sync.Mutex
	projSeq map[string]int64

	// summarize performs one summary-memory model call (nil disables the
	// feature, e.g. in tests that build the host via struct literal). See
	// summary_memory.go.
	summarize func(ctx context.Context, prompt string) (string, error)
	// summaryFlight keeps at most one summary update in flight per room.
	summaryFlight sync.Map
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
	settingsService *settings.Service,
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

	// Resolve each claudecode task's config the same way single-chat does: overlay
	// the assigned bot's provider_ext.claudecode (api_key/auth_token/base_url AND
	// model), falling back to the 通用设置 anthropic provider. Carrying the bot's
	// model is what makes Claude Code actually produce output against DeepSeek.
	claudeBotConfigResolver := func(ctx context.Context, agentID string) providers.ClaudeCodeConfig {
		var out providers.ClaudeCodeConfig
		botID := strings.TrimPrefix(strings.TrimSpace(agentID), "bot:")
		if settingsService != nil && botID != "" {
			if st, err := settingsService.GetBot(ctx, botID); err == nil {
				if ext, ok := st.ProviderExt["claudecode"]; ok {
					if b, mErr := json.Marshal(ext); mErr == nil {
						_ = json.Unmarshal(b, &out)
					}
				}
			}
		}
		// Fallback: 通用设置 anthropic provider credentials (no model).
		if out.APIKey == "" && out.AuthToken == "" && queries != nil {
			if creds, err := globalproviders.ResolveCredentialsForFramework(ctx, queries, "claudecode"); err == nil {
				out.APIKey = creds.APIKey
				if out.BaseURL == "" {
					out.BaseURL = creds.BaseURL
				}
			}
		}
		return out
	}

	claudeProvider := providers.NewClaudeCodeProvider(provCfg.ClaudeCode, resolver, store, nil, claudeBotConfigResolver, log)
	codexProvider := providers.NewCodexProvider(provCfg.Codex, resolver, store, nil, log)
	memohProvider := providers.NewMemohProvider(memohRunner, store, log)

	registry := orch.NewProviderRegistry(
		claudeProvider,
		codexProvider,
		memohProvider,
		orch.NoopProvider{},
	)

	// Lazy planner: the LLM model is resolved on first use (and re-probed at
	// most once a minute while absent), so configuring the first chat model
	// after boot upgrades planning without a restart.
	planner := newLazyPlanner(func(ctx context.Context) orch.Planner {
		model := resolvePlannerModel(ctx, queries, log)
		if model == nil {
			return nil
		}
		return newLLMPlanner(model, orch.NewRulePlanner(), log)
	}, orch.NewRulePlanner(), plannerResolveRetryInterval, log)
	orchestrator := orch.NewService(store, planner, registry, log, orch.Config{
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
		// Summary memory reuses the planner's model resolution (first enabled
		// chat model, chat-completions preferred) per call; calls are rare —
		// only when the history window actually overflows.
		summarize: func(ctx context.Context, prompt string) (string, error) {
			model := resolvePlannerModel(ctx, queries, log)
			if model == nil {
				return "", errors.New("no enabled chat model for summary memory")
			}
			return sdk.GenerateText(ctx,
				sdk.WithModel(model),
				sdk.WithSystem(summarySystemPrompt),
				sdk.WithMessages([]sdk.Message{sdk.UserMessage(prompt)}),
				sdk.WithMaxTokens(512),
				sdk.WithTemperature(0.2),
			)
		},
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
	// Agent display names ride in run metadata so prompt builders can label
	// upstream outputs "[前端]" instead of an opaque "[bot:<uuid>]".
	if _, exists := metadata["agent_names"]; !exists {
		names := make(map[string]any, len(agents))
		for _, a := range agents {
			if n := strings.TrimSpace(a.Name); n != "" && strings.TrimSpace(a.ID) != "" {
				names[a.ID] = n
			}
		}
		if len(names) > 0 {
			metadata["agent_names"] = names
		}
	}
	if _, exists := metadata["room_history"]; !exists {
		// Context layering, long-term → short-term: pinned messages (user-curated,
		// permanent), then the rolling summary of conversation that scrolled out
		// of the window (summary memory, auto-generated), then the budgeted
		// recent-history window itself.
		pinned := s.pinnedRoomContext(ctx, ownerUserID, roomID, room)
		summary := roomSummaryBlock(room.Metadata)
		win := s.recentRoomHistoryWindow(ctx, ownerUserID, roomID, 20)
		if combined := joinContextBlocks(pinned, summary, win.transcript); combined != "" {
			metadata["room_history"] = combined
		}
		// Fold what the budget pushed out into the rolling summary for *future*
		// runs — async, so run start never waits on a model call.
		if win.truncated && summaryMemoryEnabled(room.Metadata) {
			s.kickRoomSummary(ctx, ownerUserID, roomID, win.dropped)
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
	s.projectRun(ctx, snapshot)
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
	s.projectRun(ctx, reconciled)
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
		s.projectRun(ctx, snapshot)
		out = append(out, snapshot)
	}
	return out, nil
}

// RunBackgroundReconciler drives active runs to completion without depending on
// frontend polling. It reconciles once immediately — the crash self-heal path:
// failTimedOutTasks marks attempts started in a previous process lifetime as
// interrupted and retries them — and then on every tick, so runs whose room is
// not open in any browser still make progress. It blocks until ctx is cancelled.
//
// Concurrent reconciles from HTTP handlers are safe: the engine's transitions
// are state-machine-guarded and projection is idempotent at the insert layer.
func (s *OrchestratorService) RunBackgroundReconciler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		s.reconcileActiveRunsOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// reconcileActiveRunsOnce advances every non-terminal run and projects its new
// events. Per-run errors are logged and skipped so one stuck run cannot stall
// the others.
func (s *OrchestratorService) reconcileActiveRunsOnce(ctx context.Context) {
	runs, err := s.orch.ListActiveRuns(ctx)
	if err != nil {
		s.log.Error("background reconcile: list active runs failed", slog.Any("error", err))
		return
	}
	for _, run := range runs {
		if ctx.Err() != nil {
			return
		}
		snapshot, err := s.orch.ReconcileRun(ctx, run.ID)
		if err != nil {
			s.log.Error("background reconcile: run failed",
				slog.String("run_id", run.ID),
				slog.Any("error", err),
			)
			continue
		}
		s.projectRun(ctx, snapshot)
	}
}

// buildPlanner returns an LLM-backed planner (with rule-planner fallback) when
// Anthropic credentials are available, otherwise the deterministic rule planner.
// It logs which path is active so "the orchestrator is intelligent" is verifiable.
// Context size budgets, in runes (conservative token proxy). The rendered
// history is persisted in run metadata and prepended to the planner prompt and
// every sub-task CLI prompt, so it must stay bounded no matter how large
// individual room messages are (projected agent outputs can be full code dumps).
const (
	historyMessageMaxChars = 1500
	historyTotalMaxChars   = 12000
	pinnedMessageMaxChars  = 1500
	pinnedTotalMaxChars    = 6000
)

// historyWindow is the budgeted recent-history view of a room: the rendered
// transcript plus what the budget pushed out (feedstock for summary memory).
type historyWindow struct {
	transcript string
	truncated  bool
	// dropped holds the fetched messages excluded from the transcript by the
	// character budget, oldest→newest.
	dropped []Message
}

// recentRoomHistoryWindow renders the room's most recent messages as a
// plain-text transcript (oldest→newest) for use as orchestrator run context.
// The window is budgeted, not just counted: each message is clamped and the
// transcript keeps the newest messages that fit historyTotalMaxChars, dropping
// the oldest first. Best-effort: returns a zero window on any error.
func (s *OrchestratorService) recentRoomHistoryWindow(ctx context.Context, ownerUserID, roomID string, limit int32) historyWindow {
	resp, err := s.rooms.ListMessages(ctx, ownerUserID, roomID, limit)
	if err != nil || len(resp.Items) == 0 {
		return historyWindow{}
	}
	win := historyWindow{}
	lines := make([]string, 0, len(resp.Items))
	total := 0
	for i := len(resp.Items) - 1; i >= 0; i-- { // newest → oldest
		m := resp.Items[i]
		body := strings.TrimSpace(m.Body)
		if body == "" {
			continue
		}
		body = providers.ClampMiddle(body, historyMessageMaxChars)
		name := strings.TrimSpace(m.SenderName)
		if name == "" {
			name = strings.TrimSpace(m.SenderType)
		}
		line := name + "：" + body
		if total+len([]rune(line)) > historyTotalMaxChars && len(lines) > 0 {
			win.truncated = true
			// This message and everything older fell out of the window.
			win.dropped = append(win.dropped, resp.Items[:i+1]...)
			break
		}
		lines = append(lines, line)
		total += len([]rune(line)) + 1
	}
	// The fetch window itself may have cut off older messages.
	if limit > 0 && len(resp.Items) >= int(limit) {
		win.truncated = true
	}
	// Collected newest-first; emit oldest→newest.
	var b strings.Builder
	// Tell the agents the transcript is incomplete rather than letting them
	// assume this is the whole conversation (pins still cover older key facts).
	if win.truncated && len(lines) > 0 {
		fmt.Fprintf(&b, "（更早的对话因长度限制未包含，以下仅为最近 %d 条消息）\n", len(lines))
	}
	for i := len(lines) - 1; i >= 0; i-- {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}
	win.transcript = strings.TrimSpace(b.String())
	return win
}

// recentRoomHistory is the transcript-only view of recentRoomHistoryWindow.
func (s *OrchestratorService) recentRoomHistory(ctx context.Context, ownerUserID, roomID string, limit int32) string {
	return s.recentRoomHistoryWindow(ctx, ownerUserID, roomID, limit).transcript
}

// pinnedRoomContext renders the room's user-pinned messages (room.metadata
// .pinned_message_ids) as a labelled long-term-context block. Pins are resolved
// by id so they stay in context no matter how far they scroll out of the
// recent-history window — that persistence is the point of pinning.
// Best-effort: returns "" when there are no pins or on any lookup error.
func (s *OrchestratorService) pinnedRoomContext(ctx context.Context, ownerUserID, roomID string, room Room) string {
	ids := pinnedMessageIDs(room.Metadata)
	if len(ids) == 0 {
		return ""
	}
	var b strings.Builder
	for _, id := range ids {
		m, err := s.rooms.GetMessage(ctx, ownerUserID, roomID, id)
		if err != nil {
			continue
		}
		body := strings.TrimSpace(m.Body)
		if body == "" {
			continue
		}
		body = providers.ClampMiddle(body, pinnedMessageMaxChars)
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
	return "用户置顶的关键信息（长期上下文，务必优先考虑）：\n" + providers.ClampMiddle(pinned, pinnedTotalMaxChars)
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

// resolvePlannerModel builds the LLM planner's model from the user's first
// enabled chat model and its provider — the same proven path the chat/ds runtime
// uses (e.g. DeepSeek via openai-completions), instead of a hard-coded
// anthropic-messages client. Returns nil (→ rule planner) when no chat model is
// configured or credentials can't be resolved.
func resolvePlannerModel(ctx context.Context, queries dbstore.Queries, log *slog.Logger) *sdk.Model {
	if queries == nil {
		return nil
	}
	modelsSvc := models.NewService(log, queries)
	chatModels, err := modelsSvc.ListEnabledByType(ctx, models.ModelTypeChat)
	if err != nil || len(chatModels) == 0 {
		return nil
	}
	provSvc := globalproviders.NewService(log, queries, "")
	// Prefer a plain chat-completions provider (e.g. DeepSeek openai-completions),
	// then anthropic-messages. Skip openai-responses / codex, which 404 against a
	// DeepSeek-compatible endpoint (the /responses API isn't supported there).
	rank := map[string]int{
		string(models.ClientTypeOpenAICompletions): 2,
		string(models.ClientTypeAnthropicMessages): 1,
	}
	var best *sdk.Model
	bestRank := 0
	for _, cm := range chatModels {
		provUUID, perr := db.ParseUUID(cm.ProviderID)
		if perr != nil {
			continue
		}
		provider, perr := queries.GetProviderByID(ctx, provUUID)
		if perr != nil {
			continue
		}
		r := rank[provider.ClientType]
		if r <= bestRank {
			continue
		}
		creds, cerr := provSvc.ResolveModelCredentials(ctx, provider)
		if cerr != nil {
			continue
		}
		best = models.NewSDKChatModel(models.SDKModelConfig{
			ModelID:    cm.ModelID,
			ClientType: provider.ClientType,
			APIKey:     creds.APIKey,
			BaseURL:    globalproviders.ProviderConfigString(provider, "base_url"),
		})
		bestRank = r
		if r == 2 {
			break
		}
	}
	if best != nil {
		log.Info("agenthub planner: resolved LLM model", slog.Int("provider_rank", bestRank))
	}
	return best
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
		provider, name, botCaps := s.resolveAgentProvider(ctx, id)
		caps := defaultCaps
		switch provider {
		case "claudecode", "codex":
			caps = []string{"plan", "code", "test", "review", "edit", "exec"}
		case "memoh":
			caps = []string{"plan", "analysis", "review", "chat"}
		}
		// The provider baseline says what the runtime can do; the bot's
		// user-configured capabilities add what this agent is *for* (e.g.
		// "前端", "文档"). Both planners read these: the rule planner keyword-
		// matches them and the LLM planner sees them in its agent roster, so
		// merging (never removing) gives scheduling more signal without
		// regressing any existing assignment.
		caps = mergeCapabilities(caps, botCaps)
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

// resolveAgentProvider maps a room agent id to its orchestrator provider name,
// display name, and the bot's user-configured capabilities. A "bot:UUID" id is
// resolved against the bot store by the bot's framework (claudecode/codex/
// memoh) so backend-derived runs (those that don't carry an explicit Agents
// list, e.g. reconcile) dispatch to the real provider instead of falling
// through to noop. Non-bot ids and lookup failures fall back to the
// id-heuristic friendlyAgentName with no extra capabilities.
func (s *OrchestratorService) resolveAgentProvider(ctx context.Context, agentID string) (provider string, name string, capabilities []string) {
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
				return "claudecode", display, bot.Capabilities
			case bots.FrameworkCodex:
				return "codex", display, bot.Capabilities
			case bots.FrameworkMemoh:
				return "memoh", display, bot.Capabilities
			}
		}
	}
	provider, name = friendlyAgentName(id)
	return provider, name, nil
}

// mergeCapabilities appends the bot's user-configured capabilities to the
// provider baseline, deduplicating case-insensitively and keeping order
// (baseline first, custom extras after).
func mergeCapabilities(baseline, extra []string) []string {
	if len(extra) == 0 {
		return baseline
	}
	seen := make(map[string]struct{}, len(baseline)+len(extra))
	out := make([]string, 0, len(baseline)+len(extra))
	for _, group := range [][]string{baseline, extra} {
		for _, c := range group {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			key := strings.ToLower(c)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, c)
		}
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
