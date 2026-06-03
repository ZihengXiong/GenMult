package bots

import "testing"

func TestNormalizeFramework(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: FrameworkMemoh},
		{in: "  ", want: FrameworkMemoh},
		{in: "memoh", want: FrameworkMemoh},
		{in: "claudecode", want: FrameworkClaudeCode},
		{in: "codex", want: FrameworkCodex},
		{in: " codex ", want: FrameworkCodex},
		{in: "gpt", wantErr: true},
	}
	for _, tc := range cases {
		got, err := normalizeFramework(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeFramework(%q): expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeFramework(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeFramework(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeFrameworkValue(t *testing.T) {
	if got := normalizeFrameworkValue(""); got != FrameworkMemoh {
		t.Errorf("empty: got %q, want %q", got, FrameworkMemoh)
	}
	if got := normalizeFrameworkValue("claudecode"); got != FrameworkClaudeCode {
		t.Errorf("claudecode: got %q, want %q", got, FrameworkClaudeCode)
	}
}
