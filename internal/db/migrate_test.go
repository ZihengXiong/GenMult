package db

import (
	"testing"

	"github.com/ZihengXiong/GenMult/internal/config"
)

func TestRunMigrateUnknownCommand(t *testing.T) {
	cfg := config.PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "memoh",
		Password: "secret",
		Database: "memoh",
		SSLMode:  "disable",
	}
	err := RunMigrate(nil, cfg, nil, "invalid", nil)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}
