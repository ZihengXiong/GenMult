package agenthub

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// seedSequentialMessages inserts n user messages "msg-1"…"msg-n" with strictly
// increasing created_at (CURRENT_TIMESTAMP only has 1s resolution, so rapid
// inserts would otherwise tie and sort arbitrarily by random UUID).
func seedSequentialMessages(t *testing.T, conn *sql.DB, rooms *Service, roomID string, n int) []Message {
	t.Helper()
	ctx := context.Background()
	out := make([]Message, 0, n)
	for i := 1; i <= n; i++ {
		msg, err := rooms.CreateMessage(ctx, testOwnerID, roomID, CreateMessageRequest{
			SenderType: "user",
			SenderName: "Me",
			Body:       fmt.Sprintf("msg-%d", i),
		})
		if err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
		if _, err := conn.ExecContext(ctx,
			`UPDATE agent_hub_room_messages SET created_at = ? WHERE id = ?`,
			fmt.Sprintf("2030-01-01 10:%02d:%02d", i/60, i%60), msg.ID,
		); err != nil {
			t.Fatalf("set created_at for message %d: %v", i, err)
		}
		out = append(out, msg)
	}
	return out
}

// TestListMessagesReturnsLatestWindow guards the recent-history semantics: the
// limit must keep the newest N messages (in chronological order), not freeze
// the window at the room's oldest N — that breakage silently cut multi-turn
// context for the planner once a room outgrew the window.
func TestListMessagesReturnsLatestWindow(t *testing.T) {
	ctx := context.Background()
	conn, rooms := newProjectionTestRooms(t)
	room := createProjectionTestRoom(t, rooms)
	seedSequentialMessages(t, conn, rooms, room.ID, 25)

	resp, err := rooms.ListMessages(ctx, testOwnerID, room.ID, 10)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(resp.Items) != 10 {
		t.Fatalf("message count = %d, want 10", len(resp.Items))
	}
	for i, m := range resp.Items {
		want := fmt.Sprintf("msg-%d", 16+i) // newest 10 of 25, oldest→newest
		if m.Body != want {
			t.Fatalf("item %d body = %q, want %q", i, m.Body, want)
		}
	}
}

// TestPinnedRoomContextSurvivesScrollOut guards the long-term-memory promise of
// pinning: a pinned message must enter run context even after it has scrolled
// far out of the recent-history window (pins are resolved by id, not by
// scanning a window).
func TestPinnedRoomContextSurvivesScrollOut(t *testing.T) {
	ctx := context.Background()
	conn, rooms := newProjectionTestRooms(t)
	room := createProjectionTestRoom(t, rooms)

	pinned, err := rooms.CreateMessage(ctx, testOwnerID, room.ID, CreateMessageRequest{
		SenderType: "user",
		SenderName: "Me",
		Body:       "必须用 Rust 实现",
	})
	if err != nil {
		t.Fatalf("create pinned message: %v", err)
	}
	// Pin the timestamp before the seeded window so the message deterministically
	// scrolls out (CURRENT_TIMESTAMP would race with the fixed seed times).
	if _, err := conn.ExecContext(ctx,
		`UPDATE agent_hub_room_messages SET created_at = '2030-01-01 09:00:00' WHERE id = ?`, pinned.ID,
	); err != nil {
		t.Fatalf("set pinned created_at: %v", err)
	}
	seedSequentialMessages(t, conn, rooms, room.ID, 30)

	room, err = rooms.Update(ctx, testOwnerID, room.ID, UpsertRoomRequest{
		Name:     room.Name,
		Metadata: map[string]any{"pinned_message_ids": []string{pinned.ID}},
	})
	if err != nil {
		t.Fatalf("update room metadata: %v", err)
	}

	host := &OrchestratorService{
		rooms:   rooms,
		log:     slog.New(slog.DiscardHandler),
		projSeq: make(map[string]int64),
	}
	got := host.pinnedRoomContext(ctx, testOwnerID, room.ID, room)
	if !strings.Contains(got, "必须用 Rust 实现") {
		t.Fatalf("pinned context missing pinned body, got %q", got)
	}

	// And the recent window indeed no longer contains the pinned message,
	// which is exactly why pins must resolve by id.
	recent := host.recentRoomHistory(ctx, testOwnerID, room.ID, 20)
	if strings.Contains(recent, "必须用 Rust 实现") {
		t.Fatalf("expected pinned message to be outside the recent window, got %q", recent)
	}
	if !strings.Contains(recent, "msg-30") {
		t.Fatalf("recent history missing newest message, got %q", recent)
	}
}

// TestRecentRoomHistoryBudget guards the context budget: the rendered history
// must clamp oversized messages and keep the newest messages within the total
// budget (dropping the oldest first), so run metadata and CLI prompts stay
// bounded no matter how large projected agent outputs get.
func TestRecentRoomHistoryBudget(t *testing.T) {
	ctx := context.Background()
	conn, rooms := newProjectionTestRooms(t)
	room := createProjectionTestRoom(t, rooms)

	host := &OrchestratorService{
		rooms:   rooms,
		log:     slog.New(slog.DiscardHandler),
		projSeq: make(map[string]int64),
	}

	// One oversized agent dump plus a tail of normal messages.
	msgs := seedSequentialMessages(t, conn, rooms, room.ID, 12)
	huge := strings.Repeat("码", historyMessageMaxChars*5)
	if _, err := conn.ExecContext(ctx,
		`UPDATE agent_hub_room_messages SET body = ? WHERE id = ?`, huge, msgs[5].ID,
	); err != nil {
		t.Fatalf("inflate message: %v", err)
	}

	got := host.recentRoomHistory(ctx, testOwnerID, room.ID, 20)
	if runes := len([]rune(got)); runes > historyTotalMaxChars+historyMessageMaxChars {
		t.Fatalf("history length = %d runes, want bounded", runes)
	}
	if !strings.Contains(got, "msg-12") {
		t.Fatalf("history must keep the newest message, got %q", got)
	}
	if !strings.Contains(got, "已截断") {
		t.Fatalf("oversized message should be clamped with a marker")
	}

	// With many oversized messages the oldest must drop out entirely.
	for _, m := range msgs {
		if _, err := conn.ExecContext(ctx,
			`UPDATE agent_hub_room_messages SET body = ? WHERE id = ?`,
			strings.Repeat("长", historyMessageMaxChars*2)+" "+m.Body, m.ID,
		); err != nil {
			t.Fatalf("inflate message: %v", err)
		}
	}
	got = host.recentRoomHistory(ctx, testOwnerID, room.ID, 20)
	if runes := len([]rune(got)); runes > historyTotalMaxChars+historyMessageMaxChars {
		t.Fatalf("history length = %d runes, want bounded", runes)
	}
	if strings.Contains(got, "msg-1\n") || strings.HasPrefix(got, "Me：msg-1") {
		t.Fatalf("oldest message should be dropped under budget pressure")
	}
	// Dropped history must be announced so agents don't mistake the window for
	// the whole conversation.
	if !strings.Contains(got, "更早的对话因长度限制未包含") {
		t.Fatalf("expected truncation notice when older messages are dropped, got %q", got)
	}
}

// TestRecentRoomHistoryNoTruncationNotice: a small conversation that fits the
// window completely must not carry the truncation notice.
func TestRecentRoomHistoryNoTruncationNotice(t *testing.T) {
	ctx := context.Background()
	conn, rooms := newProjectionTestRooms(t)
	room := createProjectionTestRoom(t, rooms)
	_ = conn

	host := &OrchestratorService{
		rooms:   rooms,
		log:     slog.New(slog.DiscardHandler),
		projSeq: make(map[string]int64),
	}
	seedSequentialMessages(t, conn, rooms, room.ID, 3)

	got := host.recentRoomHistory(ctx, testOwnerID, room.ID, 20)
	if got == "" {
		t.Fatal("expected non-empty history")
	}
	if strings.Contains(got, "更早的对话因长度限制未包含") {
		t.Fatalf("small history must not claim truncation, got %q", got)
	}
}

// TestMergeCapabilities: provider baseline stays first, user-configured extras
// append, duplicates (case-insensitive) and blanks are dropped.
func TestMergeCapabilities(t *testing.T) {
	baseline := []string{"plan", "code", "test"}
	got := mergeCapabilities(baseline, []string{" 前端 ", "Code", "", "文档"})
	want := []string{"plan", "code", "test", "前端", "文档"}
	if len(got) != len(want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("merged = %v, want %v", got, want)
		}
	}
	if out := mergeCapabilities(baseline, nil); &out[0] != &baseline[0] {
		t.Fatalf("no extras should return baseline as-is")
	}
}

// TestAgentsFromRoomNonBotDefaults: non-bot ids keep provider-derived caps.
func TestAgentsFromRoomNonBotDefaults(t *testing.T) {
	host := &OrchestratorService{log: slog.New(slog.DiscardHandler), projSeq: make(map[string]int64)}
	agents := host.agentsFromRoom(context.Background(), Room{AgentIDs: []string{"claude-code", "my-bot"}})
	if len(agents) != 2 {
		t.Fatalf("agents = %d, want 2", len(agents))
	}
	if agents[0].ProviderName != "claudecode" || len(agents[0].Capabilities) != 6 {
		t.Fatalf("claude agent caps = %v", agents[0].Capabilities)
	}
	if agents[1].ProviderName != "noop" || len(agents[1].Capabilities) != 4 {
		t.Fatalf("fallback agent caps = %v", agents[1].Capabilities)
	}
}
