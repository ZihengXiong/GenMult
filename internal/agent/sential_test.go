package agent

import (
	"sync"
	"testing"
)

func TestToolLoopGuardNilReceiver(t *testing.T) {
	var guard *ToolLoopGuard

	result := guard.Inspect(ToolLoopInput{
		ToolName: "search",
		Input: map[string]any{
			"query": "memoh",
		},
	})

	if result.Hash == "" {
		t.Fatal("expected hash for nil receiver")
	}
	if result.Warn {
		t.Fatal("did not expect warning for nil receiver")
	}
	if result.Abort {
		t.Fatal("did not expect abort for nil receiver")
	}
}

func TestToolLoopGuardConcurrentInspectAndReset(t *testing.T) {
	guard := NewToolLoopGuard(2, 1)
	input := ToolLoopInput{
		ToolName: "web_search",
		Input: map[string]any{
			"query":     "memoh logs",
			"requestId": "volatile",
		},
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				result := guard.Inspect(input)
				if result.Hash == "" {
					t.Error("expected non-empty hash")
					return
				}
				if (i+j)%25 == 0 {
					guard.Reset()
				}
			}
		}(i)
	}
	wg.Wait()
}

// callsFor builds a ToolLoopInput for a synthetic tool key.
func cycleCall(key string) ToolLoopInput {
	return ToolLoopInput{ToolName: key, Input: map[string]any{"arg": key}}
}

// TestToolLoopGuardDetectsAlternatingCycle: an A/B livelock (identical args
// each time) must warn once and then abort, even though no single call repeats
// consecutively (so the period-1 repeat counter never fires).
func TestToolLoopGuardDetectsAlternatingCycle(t *testing.T) {
	guard := NewToolLoopGuard(0, 0)

	warned := false
	for i := 0; i < 64; i++ {
		key := "a"
		if i%2 == 1 {
			key = "b"
		}
		result := guard.Inspect(cycleCall(key))
		if result.Abort {
			if !warned {
				t.Fatal("abort before the grace warning")
			}
			if result.CyclePeriod != 2 {
				t.Fatalf("expected cycle period 2, got %d", result.CyclePeriod)
			}
			if i >= 40 {
				t.Fatalf("abort too late: call %d", i)
			}
			return
		}
		if result.Warn {
			warned = true
			if result.CyclePeriod != 2 {
				t.Fatalf("warn should carry cycle period 2, got %d", result.CyclePeriod)
			}
		}
	}
	t.Fatal("alternating cycle was never aborted")
}

// TestToolLoopGuardDetectsPeriodThreeCycle: A/B/C repeated must abort too.
func TestToolLoopGuardDetectsPeriodThreeCycle(t *testing.T) {
	guard := NewToolLoopGuard(0, 0)
	keys := []string{"a", "b", "c"}
	for i := 0; i < 96; i++ {
		result := guard.Inspect(cycleCall(keys[i%3]))
		if result.Abort {
			if result.CyclePeriod != 3 {
				t.Fatalf("expected cycle period 3, got %d", result.CyclePeriod)
			}
			return
		}
	}
	t.Fatal("period-3 cycle was never aborted")
}

// TestToolLoopGuardNoCycleOnVariedCalls: distinct arguments every call (normal
// forward progress) must never warn or abort.
func TestToolLoopGuardNoCycleOnVariedCalls(t *testing.T) {
	guard := NewToolLoopGuard(0, 0)
	for i := 0; i < 200; i++ {
		key := "step"
		result := guard.Inspect(ToolLoopInput{ToolName: key, Input: map[string]any{"n": float64(i)}})
		if result.Warn || result.Abort {
			t.Fatalf("varied calls must not trigger the guard (call %d)", i)
		}
	}
}

// TestToolLoopGuardNoCycleOnInterleavedWork: a repeating pair broken up by
// other calls (real workflows revisit files) must not trigger cycle detection.
func TestToolLoopGuardNoCycleOnInterleavedWork(t *testing.T) {
	guard := NewToolLoopGuard(0, 0)
	for i := 0; i < 40; i++ {
		for _, key := range []string{"read", "edit"} {
			if result := guard.Inspect(cycleCall(key)); result.Warn || result.Abort {
				// read/edit pairs alone *are* a cycle; the breaker below must
				// keep resetting the window before detection trips.
				t.Fatalf("interleaved work misdetected as cycle (iteration %d)", i)
			}
		}
		if result := guard.Inspect(ToolLoopInput{ToolName: "exec", Input: map[string]any{"step": float64(i)}}); result.Warn || result.Abort {
			t.Fatalf("breaker call misdetected (iteration %d)", i)
		}
	}
}

// TestToolLoopGuardIdenticalRepeatStillAborts: the period-1 path must be
// unaffected by the cycle detector (uniform windows are excluded from it).
func TestToolLoopGuardIdenticalRepeatStillAborts(t *testing.T) {
	guard := NewToolLoopGuard(0, 0)
	warned, aborted := false, false
	for i := 0; i < 32 && !aborted; i++ {
		result := guard.Inspect(cycleCall("same"))
		if result.Warn {
			warned = true
			if result.CyclePeriod != 0 {
				t.Fatalf("identical repeats must warn via the period-1 path, got cycle period %d", result.CyclePeriod)
			}
		}
		aborted = result.Abort
	}
	if !warned || !aborted {
		t.Fatalf("identical repeats: warned=%v aborted=%v", warned, aborted)
	}
}
