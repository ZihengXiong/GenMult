package db

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/config"
)

func TestSQLiteFileDSNPreservesWindowsPath(t *testing.T) {
	cfg := config.SQLiteConfig{
		DSN: "sqlite://C:\\Users\\Dustella\\AppData\\Roaming\\Memoh\\data\\local\\memoh.db?_busy_timeout=5000&_journal_mode=WAL",
	}

	got := SQLiteFileDSN(cfg)
	if !strings.HasPrefix(got, "C:\\Users\\Dustella\\AppData\\Roaming\\Memoh\\data\\local\\memoh.db?") {
		t.Fatalf("SQLiteFileDSN() = %q, want Windows file path prefix", got)
	}
	if !strings.Contains(got, "_pragma=busy_timeout%285000%29") {
		t.Fatalf("SQLiteFileDSN() = %q, want busy_timeout pragma", got)
	}
	if !strings.Contains(got, "_pragma=journal_mode%28WAL%29") {
		t.Fatalf("SQLiteFileDSN() = %q, want WAL pragma", got)
	}
}

func TestSQLiteFileDSNPreservesUnixPath(t *testing.T) {
	cfg := config.SQLiteConfig{
		DSN: "sqlite:///tmp/memoh.db?_busy_timeout=5000&_journal_mode=WAL",
	}

	got := SQLiteFileDSN(cfg)
	if !strings.HasPrefix(got, "/tmp/memoh.db?") {
		t.Fatalf("SQLiteFileDSN() = %q, want Unix file path prefix", got)
	}
	if !strings.Contains(got, "_pragma=busy_timeout%285000%29") {
		t.Fatalf("SQLiteFileDSN() = %q, want busy_timeout pragma", got)
	}
	if !strings.Contains(got, "_pragma=journal_mode%28WAL%29") {
		t.Fatalf("SQLiteFileDSN() = %q, want WAL pragma", got)
	}
}
