package agent

import (
	"log/slog"

	"github.com/ZihengXiong/GenMult/internal/workspace/bridge"
)

// Deps holds all service dependencies for the Agent.
type Deps struct {
	BridgeProvider bridge.Provider
	Logger         *slog.Logger
}
