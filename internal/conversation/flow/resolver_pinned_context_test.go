package flow

import "testing"

func TestPinnedContextFromMetadata(t *testing.T) {
	if got := pinnedContextFromMetadata(nil); got != "" {
		t.Errorf("nil metadata: got %q", got)
	}
	if got := pinnedContextFromMetadata(map[string]any{}); got != "" {
		t.Errorf("missing key: got %q", got)
	}
	if got := pinnedContextFromMetadata(map[string]any{"pinned_context": "not-an-array"}); got != "" {
		t.Errorf("non-array: got %q", got)
	}

	got := pinnedContextFromMetadata(map[string]any{
		"pinned_context": []any{"记住用 TypeScript", "  ", 42, "风格简洁"},
	})
	want := "- 记住用 TypeScript\n- 风格简洁"
	if got != want {
		t.Errorf("pinned list: got %q, want %q", got, want)
	}
}
