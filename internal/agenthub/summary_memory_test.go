package agenthub

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func summaryTestHost(t *testing.T) (*OrchestratorService, *Service, Room) {
	t.Helper()
	_, rooms := newProjectionTestRooms(t)
	room := createProjectionTestRoom(t, rooms)
	host := &OrchestratorService{
		rooms:   rooms,
		log:     slog.New(slog.DiscardHandler),
		projSeq: make(map[string]int64),
	}
	return host, rooms, room
}

func summaryMsg(name, body string, at time.Time) Message {
	return Message{SenderName: name, SenderType: "user", Body: body, CreatedAt: at}
}

func TestUpdateRoomSummaryFoldsDroppedMessages(t *testing.T) {
	ctx := context.Background()
	host, rooms, room := summaryTestHost(t)

	var calls atomic.Int32
	var lastPrompt atomic.Value
	host.summarize = func(_ context.Context, prompt string) (string, error) {
		calls.Add(1)
		lastPrompt.Store(prompt)
		return "  第一版摘要：讨论了部署方案。  ", nil
	}

	base := time.Date(2030, 1, 1, 9, 0, 0, 0, time.UTC)
	dropped := []Message{
		summaryMsg("Alice", "我们用 docker compose 部署", base),
		summaryMsg("Bob", "数据库选 sqlite", base.Add(time.Minute)),
	}
	if err := host.updateRoomSummary(ctx, testOwnerID, room.ID, dropped); err != nil {
		t.Fatalf("updateRoomSummary: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 summarize call, got %d", calls.Load())
	}
	prompt, _ := lastPrompt.Load().(string)
	for _, want := range []string{"Alice：我们用 docker compose 部署", "Bob：数据库选 sqlite", "新增对话"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %q", want, prompt)
		}
	}
	if strings.Contains(prompt, "既有摘要") {
		t.Fatalf("first fold must not claim a previous summary: %q", prompt)
	}

	got, err := rooms.Get(ctx, testOwnerID, room.ID)
	if err != nil {
		t.Fatalf("get room: %v", err)
	}
	if s, _ := got.Metadata[metaKeyHistorySummary].(string); s != "第一版摘要：讨论了部署方案。" {
		t.Fatalf("stored summary = %q", s)
	}
	if through := summaryThroughAt(got.Metadata); !through.Equal(base.Add(time.Minute)) {
		t.Fatalf("through = %v, want %v", through, base.Add(time.Minute))
	}

	// Re-folding the same dropped set must be a no-op (already summarized).
	if err := host.updateRoomSummary(ctx, testOwnerID, room.ID, dropped); err != nil {
		t.Fatalf("idempotent fold: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("already-summarized messages must not trigger a call, got %d", calls.Load())
	}

	// A newer dropped message folds incrementally on top of the previous summary.
	host.summarize = func(_ context.Context, prompt string) (string, error) {
		calls.Add(1)
		lastPrompt.Store(prompt)
		if !strings.Contains(prompt, "既有摘要") || !strings.Contains(prompt, "第一版摘要") {
			t.Errorf("incremental fold must carry the previous summary: %q", prompt)
		}
		return "第二版摘要", nil
	}
	newer := []Message{summaryMsg("Alice", "前端框架定 Vue", base.Add(2*time.Minute))}
	if err := host.updateRoomSummary(ctx, testOwnerID, room.ID, newer); err != nil {
		t.Fatalf("incremental fold: %v", err)
	}
	got, _ = rooms.Get(ctx, testOwnerID, room.ID)
	if s, _ := got.Metadata[metaKeyHistorySummary].(string); s != "第二版摘要" {
		t.Fatalf("stored summary after incremental fold = %q", s)
	}
}

func TestUpdateRoomSummaryRespectsOptOut(t *testing.T) {
	ctx := context.Background()
	host, rooms, room := summaryTestHost(t)
	if _, err := rooms.MergeRoomMetadata(ctx, testOwnerID, room.ID, map[string]any{metaKeySummaryMemory: false}); err != nil {
		t.Fatalf("opt out: %v", err)
	}
	host.summarize = func(context.Context, string) (string, error) {
		t.Fatal("summarize must not be called for an opted-out room")
		return "", nil
	}
	dropped := []Message{summaryMsg("Alice", "内容", time.Date(2030, 1, 1, 9, 0, 0, 0, time.UTC))}
	if err := host.updateRoomSummary(ctx, testOwnerID, room.ID, dropped); err != nil {
		t.Fatalf("opted-out fold should no-op, got %v", err)
	}
}

func TestUpdateRoomSummaryFailureLeavesMetadataUntouched(t *testing.T) {
	ctx := context.Background()
	host, rooms, room := summaryTestHost(t)
	host.summarize = func(context.Context, string) (string, error) {
		return "", errors.New("model unavailable")
	}
	dropped := []Message{summaryMsg("Alice", "内容", time.Date(2030, 1, 1, 9, 0, 0, 0, time.UTC))}
	if err := host.updateRoomSummary(ctx, testOwnerID, room.ID, dropped); err == nil {
		t.Fatal("expected error to propagate for logging")
	}
	got, _ := rooms.Get(ctx, testOwnerID, room.ID)
	if _, has := got.Metadata[metaKeyHistorySummary]; has {
		t.Fatal("failed fold must not write a summary")
	}
}

func TestKickRoomSummarySingleFlight(t *testing.T) {
	host, _, room := summaryTestHost(t)
	var calls atomic.Int32
	release := make(chan struct{})
	done := make(chan struct{})
	host.summarize = func(context.Context, string) (string, error) {
		calls.Add(1)
		<-release
		return "摘要", nil
	}
	dropped := []Message{summaryMsg("Alice", "内容", time.Date(2030, 1, 1, 9, 0, 0, 0, time.UTC))}

	go func() {
		host.kickRoomSummary(context.Background(), testOwnerID, room.ID, dropped)
		close(done)
	}()
	<-done
	// Second kick while the first is still blocked inside summarize: skipped.
	for i := 0; i < 10; i++ {
		if calls.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	host.kickRoomSummary(context.Background(), testOwnerID, room.ID, dropped)
	close(release)
	for i := 0; i < 100; i++ {
		if _, busy := host.summaryFlight.Load(room.ID); !busy {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected single-flight to dedupe concurrent kicks, got %d calls", calls.Load())
	}
}

func TestRoomSummaryBlockAndEnabled(t *testing.T) {
	if roomSummaryBlock(map[string]any{}) != "" {
		t.Fatal("no summary → empty block")
	}
	block := roomSummaryBlock(map[string]any{metaKeyHistorySummary: "要点A；要点B"})
	if !strings.Contains(block, "早前对话摘要") || !strings.Contains(block, "要点A") {
		t.Fatalf("unexpected block: %q", block)
	}
	if !summaryMemoryEnabled(map[string]any{}) {
		t.Fatal("summary memory must default to enabled")
	}
	if summaryMemoryEnabled(map[string]any{metaKeySummaryMemory: false}) {
		t.Fatal("bool false must disable")
	}
	if summaryMemoryEnabled(map[string]any{metaKeySummaryMemory: "false"}) {
		t.Fatal("string false must disable")
	}
}
