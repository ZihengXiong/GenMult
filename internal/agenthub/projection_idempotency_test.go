package agenthub

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"testing"

	orch "github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
	"github.com/ZihengXiong/GenMult/internal/config"
	"github.com/ZihengXiong/GenMult/internal/db"
	sqlitestore "github.com/ZihengXiong/GenMult/internal/db/sqlite/store"
)

const testOwnerID = "11111111-1111-4111-8111-111111111111"

// newProjectionTestRooms builds a rooms Service over an in-memory SQLite DB
// with the real agent_hub schema, including the partial unique index from
// migration 0012 that backs insert-layer projection idempotency.
func newProjectionTestRooms(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// A pooled :memory: DSN opens a fresh empty database per connection; pin the
	// pool to one connection so every query sees the schema below.
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	execProjectionSchema(t, conn)
	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	return NewService(sqlitestore.NewQueries(store))
}

func execProjectionSchema(t *testing.T, conn *sql.DB) {
	t.Helper()
	_, err := conn.ExecContext(context.Background(), `
CREATE TABLE agent_hub_rooms (
  id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL,
  name TEXT NOT NULL,
  short_name TEXT NOT NULL DEFAULT '',
  subtitle TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  privacy TEXT NOT NULL DEFAULT '本地群',
  live TEXT NOT NULL DEFAULT '等待输入',
  accent TEXT NOT NULL DEFAULT 'bg-slate-700',
  status_class TEXT NOT NULL DEFAULT 'bg-slate-500',
  attention INTEGER NOT NULL DEFAULT 0,
  metadata TEXT NOT NULL DEFAULT '{}',
  orchestrator_agent_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE agent_hub_room_agents (
  room_id TEXT NOT NULL REFERENCES agent_hub_rooms(id) ON DELETE CASCADE,
  agent_id TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (room_id, agent_id)
);

CREATE TABLE agent_hub_room_messages (
  id TEXT PRIMARY KEY,
  room_id TEXT NOT NULL REFERENCES agent_hub_rooms(id) ON DELETE CASCADE,
  sender_id TEXT NOT NULL DEFAULT '',
  sender_type TEXT NOT NULL DEFAULT 'user',
  sender_name TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'message',
  title TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_agent_hub_room_messages_run_event
  ON agent_hub_room_messages (
    json_extract(metadata, '$.run_id'),
    json_extract(metadata, '$.event_seq')
  )
  WHERE json_extract(metadata, '$.run_id') IS NOT NULL
    AND json_extract(metadata, '$.event_seq') IS NOT NULL;
`)
	if err != nil {
		t.Fatalf("exec schema: %v", err)
	}
}

func createProjectionTestRoom(t *testing.T, rooms *Service) Room {
	t.Helper()
	room, err := rooms.Create(context.Background(), testOwnerID, UpsertRoomRequest{Name: "协作群"})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	return room
}

func TestCreateSystemMessageSuppressesDuplicateProjection(t *testing.T) {
	ctx := context.Background()
	rooms := newProjectionTestRooms(t)
	room := createProjectionTestRoom(t, rooms)

	projected := CreateMessageRequest{
		SenderType: "agent",
		SenderName: "Claude Code",
		Body:       "任务产出",
		Metadata:   map[string]any{"run_id": "run-1", "event_seq": int64(7), "event_type": "task_succeeded"},
	}
	if _, err := rooms.CreateSystemMessage(ctx, room.ID, projected); err != nil {
		t.Fatalf("first projection insert: %v", err)
	}
	if _, err := rooms.CreateSystemMessage(ctx, room.ID, projected); !errors.Is(err, ErrDuplicateMessage) {
		t.Fatalf("duplicate projection insert: got err %v, want ErrDuplicateMessage", err)
	}

	// A different event of the same run is not a duplicate.
	next := projected
	next.Metadata = map[string]any{"run_id": "run-1", "event_seq": int64(8), "event_type": "run_status_changed"}
	if _, err := rooms.CreateSystemMessage(ctx, room.ID, next); err != nil {
		t.Fatalf("next event insert: %v", err)
	}

	// Messages without run metadata (normal chat) never hit the partial index.
	plain := CreateMessageRequest{SenderType: "user", SenderName: "Me", Body: "同样的话说两遍"}
	for i := 0; i < 2; i++ {
		if _, err := rooms.CreateMessage(ctx, testOwnerID, room.ID, plain); err != nil {
			t.Fatalf("plain message %d: %v", i, err)
		}
	}

	resp, err := rooms.ListMessages(ctx, testOwnerID, room.ID, 50)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	// 1 room-created seed + 2 projected + 2 plain.
	if len(resp.Items) != 5 {
		t.Fatalf("message count = %d, want 5", len(resp.Items))
	}
}

// TestProjectRunIdempotentAcrossRestart drives a real run through the engine,
// projects it, then re-projects with an empty in-memory high-water mark (as
// after a process restart) and asserts no duplicate messages appear.
func TestProjectRunIdempotentAcrossRestart(t *testing.T) {
	ctx := context.Background()
	rooms := newProjectionTestRooms(t)
	room := createProjectionTestRoom(t, rooms)

	engine := orch.NewService(
		orch.NewMemoryStore(),
		nil, // rule planner
		orch.NewProviderRegistry(orch.NoopProvider{}),
		slog.New(slog.DiscardHandler),
		orch.Config{DispatchAsync: false},
	)
	host := &OrchestratorService{
		rooms:   rooms,
		orch:    engine,
		log:     slog.New(slog.DiscardHandler),
		projSeq: make(map[string]int64),
	}

	snapshot, err := engine.StartRun(ctx, orch.StartRunInput{
		RoomID:       room.ID,
		Objective:    "做一个待办清单工具",
		CreatedBy:    testOwnerID,
		Agents:       []orch.AgentDescriptor{{ID: "helper", ProviderName: "noop", Name: "Helper", Capabilities: []string{"plan", "code", "test", "review"}}},
		AutoDispatch: true,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if snapshot.Run.Status != orch.RunStatusCompleted {
		t.Fatalf("run status = %s, want completed", snapshot.Run.Status)
	}

	host.projectRun(ctx, snapshot)
	first, err := rooms.ListMessages(ctx, testOwnerID, room.ID, 200)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(first.Items) < 3 {
		t.Fatalf("expected projected messages, got %d", len(first.Items))
	}

	// Simulate a restart: same DB, fresh in-memory projection high-water mark.
	restarted := &OrchestratorService{
		rooms:   rooms,
		orch:    engine,
		log:     slog.New(slog.DiscardHandler),
		projMu:  sync.Mutex{},
		projSeq: make(map[string]int64),
	}
	restarted.projectRun(ctx, snapshot)

	second, err := rooms.ListMessages(ctx, testOwnerID, room.ID, 200)
	if err != nil {
		t.Fatalf("list messages after re-projection: %v", err)
	}
	if len(second.Items) != len(first.Items) {
		t.Fatalf("re-projection added messages: %d -> %d", len(first.Items), len(second.Items))
	}
}
