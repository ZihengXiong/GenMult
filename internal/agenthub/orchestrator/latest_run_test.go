package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLatestSnapshotByRoom(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, NewRulePlanner(), NewProviderRegistry(NoopProvider{}), nil, Config{DispatchAsync: false})
	ctx := context.Background()
	agents := []AgentDescriptor{{ID: "a", ProviderName: "noop", Name: "A"}}

	if _, err := svc.LatestSnapshotByRoom(ctx, "room-x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty room: expected ErrNotFound, got %v", err)
	}

	if _, err := svc.StartRun(ctx, StartRunInput{RoomID: "room-x", Objective: "first", Agents: agents}); err != nil {
		t.Fatalf("StartRun first: %v", err)
	}
	time.Sleep(2 * time.Millisecond) // guarantee a strictly later updated_at for the second run
	r2, err := svc.StartRun(ctx, StartRunInput{RoomID: "room-x", Objective: "second", Agents: agents})
	if err != nil {
		t.Fatalf("StartRun second: %v", err)
	}

	snap, err := svc.LatestSnapshotByRoom(ctx, "room-x")
	if err != nil {
		t.Fatalf("LatestSnapshotByRoom: %v", err)
	}
	if snap.Run.ID != r2.Run.ID || snap.Run.Objective != "second" {
		t.Errorf("expected latest run %q (second), got %q (%q)", r2.Run.ID, snap.Run.ID, snap.Run.Objective)
	}
	if len(snap.Tasks) == 0 {
		t.Error("expected the snapshot to include the run's tasks")
	}

	if _, err := svc.LatestSnapshotByRoom(ctx, "room-y"); !errors.Is(err, ErrNotFound) {
		t.Errorf("other room: expected ErrNotFound, got %v", err)
	}
}
