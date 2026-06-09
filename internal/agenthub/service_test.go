package agenthub

import (
	"testing"
	"time"
)

func TestSortMessagesForTimelineOrdersRunEventsBySeq(t *testing.T) {
	now := time.Date(2026, 6, 9, 15, 40, 0, 0, time.UTC)
	items := []Message{
		{ID: "c", CreatedAt: now, Metadata: map[string]any{"run_id": "run-1", "event_seq": float64(9)}, Title: "done"},
		{ID: "b", CreatedAt: now, Metadata: map[string]any{"run_id": "run-1", "event_seq": float64(6)}, Title: "dispatch"},
		{ID: "a", CreatedAt: now, Metadata: map[string]any{"run_id": "run-1", "event_seq": float64(3)}, Title: "plan"},
	}

	sortMessagesForTimeline(items)

	got := []string{items[0].Title, items[1].Title, items[2].Title}
	want := []string{"plan", "dispatch", "done"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order: got %v, want %v", got, want)
		}
	}
}
