package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/ZihengXiong/GenMult/internal/bots"
	"github.com/ZihengXiong/GenMult/internal/conversation/flow/botruntime"
	"github.com/ZihengXiong/GenMult/internal/db"
	"github.com/ZihengXiong/GenMult/internal/settings"
)

// SetBotRuntimes registers additional bot runtimes (e.g. claudecode, codex)
// alongside the default memoh runtime.
func (r *Resolver) SetBotRuntimes(runtimes ...botruntime.BotRuntime) {
	if r.runtimes == nil {
		r.runtimes = botruntime.NewRegistry(botruntime.NewMemohRuntime(r.agent))
	}
	r.runtimes.Add(runtimes...)
}

// runtimeForBot resolves the runtime backing a bot based on its framework,
// falling back to memoh when the framework is empty or unregistered.
func (r *Resolver) runtimeForBot(ctx context.Context, botID string) botruntime.BotRuntime {
	return r.runtimes.Resolve(r.loadBotFramework(ctx, botID))
}

// loadBotFramework reads a bot's framework, defaulting to memoh on any error so
// existing bots keep their current behavior.
func (r *Resolver) loadBotFramework(ctx context.Context, botID string) string {
	if r.queries == nil {
		return bots.FrameworkMemoh
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return bots.FrameworkMemoh
	}
	row, err := r.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		r.logger.Debug("failed to load bot framework",
			slog.String("bot_id", botID),
			slog.Any("error", err),
		)
		return bots.FrameworkMemoh
	}
	if framework := row.Framework; framework != "" {
		return framework
	}
	return bots.FrameworkMemoh
}

func (r *Resolver) loadBotSettings(ctx context.Context, botID string) (settings.Settings, error) {
	if r.settingsService == nil {
		return settings.Settings{}, errors.New("settings service not configured")
	}
	return r.settingsService.GetBot(ctx, botID)
}

func (r *Resolver) loadBotLoopDetectionEnabled(ctx context.Context, botID string) bool {
	if r.queries == nil {
		return false
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return false
	}
	row, err := r.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		r.logger.Debug("failed to load bot metadata for loop detection",
			slog.String("bot_id", botID),
			slog.Any("error", err),
		)
		return false
	}
	return parseLoopDetectionEnabledFromMetadata(row.Metadata)
}

func parseLoopDetectionEnabledFromMetadata(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}
	var metadata map[string]any
	if err := json.Unmarshal(payload, &metadata); err != nil || metadata == nil {
		return false
	}
	features, ok := metadata["features"].(map[string]any)
	if !ok {
		return false
	}
	loopDetection, ok := features["loop_detection"].(map[string]any)
	if !ok {
		return false
	}
	enabled, ok := loopDetection["enabled"].(bool)
	if !ok {
		return false
	}
	return enabled
}
