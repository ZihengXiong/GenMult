package agenthub

// Room conversation summary memory (Phase 1 of
// docs/plans/agenthub-room-summary-memory.md): when the recent-history window
// drops older messages under its character budget, those messages are folded
// into an incremental LLM-generated summary stored in the room's metadata, and
// the summary is injected into run context between the pinned block and the
// recent window — the ConversationSummaryBufferMemory pattern. Best-effort
// throughout: any failure only means runs keep seeing the truncation notice.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ZihengXiong/GenMult/internal/agenthub/providers"
)

const (
	// metaKeyHistorySummary holds the rolling summary text in room metadata.
	metaKeyHistorySummary = "history_summary"
	// metaKeyHistorySummaryThroughAt is the CreatedAt (RFC3339Nano) of the
	// newest message folded into the summary; older/equal messages are never
	// re-summarized.
	metaKeyHistorySummaryThroughAt = "history_summary_through_at"
	// metaKeySummaryMemory, when set to false, disables summary memory for the
	// room. Default (absent) is enabled.
	metaKeySummaryMemory = "summary_memory"

	// summaryMaxChars bounds the stored summary and its injected block.
	summaryMaxChars = 2000
	// summaryInputMessageMaxChars bounds each dropped message inside the
	// summarization prompt; summaryInputTotalMaxChars bounds the whole prompt's
	// transcript section.
	summaryInputMessageMaxChars = 800
	summaryInputTotalMaxChars   = 8000
	// summaryCallTimeout bounds one summarization model call.
	summaryCallTimeout = 45 * time.Second
)

const summarySystemPrompt = `你是群聊记忆压缩器。把"既有摘要"与"新增对话"合并为一段不超过 400 字的中文摘要，保留：每个参与者的关键结论/产出、未决问题、明确的约定与数据。不要逐句复述，不要输出任何额外说明，只输出摘要正文。`

// summaryMemoryEnabled reports whether the room opted out via metadata.
// Absent/anything-but-false means enabled.
func summaryMemoryEnabled(metadata map[string]any) bool {
	switch v := metadata[metaKeySummaryMemory].(type) {
	case bool:
		return v
	case string:
		return !strings.EqualFold(strings.TrimSpace(v), "false")
	default:
		return true
	}
}

// roomSummaryBlock renders the stored rolling summary as a labelled context
// block, or "" when none exists.
func roomSummaryBlock(metadata map[string]any) string {
	raw, _ := metadata[metaKeyHistorySummary].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return "早前对话摘要（自动生成，可能省略细节，最新进展以下方消息为准）：\n" +
		providers.ClampMiddle(raw, summaryMaxChars)
}

// summaryThroughAt parses the high-water timestamp from room metadata.
func summaryThroughAt(metadata map[string]any) time.Time {
	raw, _ := metadata[metaKeyHistorySummaryThroughAt].(string)
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

// kickRoomSummary starts a background incremental summarization of the window
// dropped messages, at most one in flight per room. ctx is detached so the
// update survives the originating request.
func (s *OrchestratorService) kickRoomSummary(ctx context.Context, ownerUserID, roomID string, dropped []Message) {
	if s.summarize == nil || len(dropped) == 0 {
		return
	}
	if _, busy := s.summaryFlight.LoadOrStore(roomID, struct{}{}); busy {
		return
	}
	bg := context.WithoutCancel(ctx)
	go func() {
		defer s.summaryFlight.Delete(roomID)
		if err := s.updateRoomSummary(bg, ownerUserID, roomID, dropped); err != nil {
			s.log.Warn("room summary update failed (best-effort)",
				slog.String("room_id", roomID), slog.Any("error", err))
		}
	}()
}

// updateRoomSummary folds the not-yet-summarized portion of dropped (ascending
// by CreatedAt) into the room's rolling summary and advances the high-water
// timestamp. Exposed as a method (rather than inlined in the goroutine) so
// tests can drive it synchronously.
func (s *OrchestratorService) updateRoomSummary(ctx context.Context, ownerUserID, roomID string, dropped []Message) error {
	if s.summarize == nil {
		return nil
	}
	// Fresh metadata: another process may have advanced the summary meanwhile.
	room, err := s.rooms.Get(ctx, ownerUserID, roomID)
	if err != nil {
		return err
	}
	if !summaryMemoryEnabled(room.Metadata) {
		return nil
	}
	through := summaryThroughAt(room.Metadata)
	prevSummary, _ := room.Metadata[metaKeyHistorySummary].(string)

	var lines []string
	total := 0
	newest := through
	for _, m := range dropped {
		if !m.CreatedAt.After(through) {
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
		line := name + "：" + providers.ClampMiddle(body, summaryInputMessageMaxChars)
		if total+len([]rune(line)) > summaryInputTotalMaxChars {
			break
		}
		lines = append(lines, line)
		total += len([]rune(line)) + 1
		if m.CreatedAt.After(newest) {
			newest = m.CreatedAt
		}
	}
	if len(lines) == 0 {
		return nil
	}

	var b strings.Builder
	if strings.TrimSpace(prevSummary) != "" {
		b.WriteString("既有摘要：\n")
		b.WriteString(providers.ClampMiddle(strings.TrimSpace(prevSummary), summaryMaxChars))
		b.WriteString("\n\n")
	}
	b.WriteString("新增对话（旧→新）：\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}

	cctx, cancel := context.WithTimeout(ctx, summaryCallTimeout)
	defer cancel()
	out, err := s.summarize(cctx, b.String())
	if err != nil {
		return err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return errors.New("summarizer returned empty output")
	}
	if _, err := s.rooms.MergeRoomMetadata(ctx, ownerUserID, roomID, map[string]any{
		metaKeyHistorySummary:          providers.ClampMiddle(out, summaryMaxChars),
		metaKeyHistorySummaryThroughAt: newest.UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return fmt.Errorf("persist room summary: %w", err)
	}
	s.log.Info("room summary advanced",
		slog.String("room_id", roomID),
		slog.Int("folded_messages", len(lines)),
		slog.Time("through", newest))
	return nil
}
