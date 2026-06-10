package providers

import (
	"context"
	"testing"

	"github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

// fakeMemohRunner drives the OnEvent callback (simulating a streaming turn) and
// returns a fixed final text, so we can assert MemohProvider.Execute surfaces
// intermediate thinking/tool events plus the terminal text event.
type fakeMemohRunner struct {
	final string
}

func (r fakeMemohRunner) RunTurn(_ context.Context, in RunTurnInput) (RunTurnResult, error) {
	if in.OnEvent != nil {
		in.OnEvent(MemohStreamEvent{Kind: "thinking", Content: "正在分析需求"})
		in.OnEvent(MemohStreamEvent{Kind: "tool", Content: "Write"})
		in.OnEvent(MemohStreamEvent{Kind: "thinking", Content: ""}) // empty → dropped
	}
	return RunTurnResult{Text: r.final}, nil
}

func TestMemohProvider_StreamsLiveEvents(t *testing.T) {
	store := &mockStore{}
	p := NewMemohProvider(fakeMemohRunner{final: "完成了待办前端"}, store, nil)

	res, err := p.Execute(context.Background(), orchestrator.ExecuteTaskRequest{
		Run:  orchestrator.Run{ID: "run-1", CreatedBy: "user-1"},
		Task: orchestrator.Task{ID: "task-1", AssignedAgentID: "bot:ds"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Output["raw_output"] != "完成了待办前端" {
		t.Errorf("unexpected output: %+v", res.Output)
	}

	// Expect: thinking event + tool event (live) + final text event; the empty
	// thinking event is dropped.
	var thinking, tool, finalText int
	for _, ev := range store.Events {
		rawType, _ := ev.Payload["raw_type"].(string)
		switch {
		case ev.Type == orchestrator.EventAgentToolCall:
			tool++
		case ev.Type == orchestrator.EventAgentOutput && rawType == "thinking":
			thinking++
		case ev.Type == orchestrator.EventAgentOutput && rawType == "text":
			finalText++
		}
	}
	if thinking != 1 || tool != 1 || finalText != 1 {
		t.Errorf("event counts: thinking=%d tool=%d finalText=%d; events=%+v", thinking, tool, finalText, store.Events)
	}
}
