package bots

import "testing"

func TestNormalizeFramework(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", input: "", want: FrameworkMemoh},
		{name: "memoh", input: "memoh", want: FrameworkMemoh},
		{name: "claudecode", input: "claudecode", want: FrameworkClaudeCode},
		{name: "codex", input: "codex", want: FrameworkCodex},
		{name: "trim", input: " claudecode ", want: FrameworkClaudeCode},
		{name: "invalid", input: "unknown", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeFramework(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
