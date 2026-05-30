package orchestrator

import "errors"

var (
	ErrInvalidInput       = errors.New("invalid orchestrator input")
	ErrNotFound           = errors.New("orchestrator record not found")
	ErrInvalidTransition  = errors.New("invalid orchestrator state transition")
	ErrProviderNotFound   = errors.New("agent provider not found")
	ErrNoExecutableTasks  = errors.New("no executable tasks")
	ErrRunTerminal        = errors.New("run is already terminal")
	ErrDuplicateClientKey = errors.New("duplicate task client key")
)

func isTerminalRunStatus(status RunStatus) bool {
	switch status {
	case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		return true
	default:
		return false
	}
}

func isTerminalTaskStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusSucceeded, TaskStatusFailed, TaskStatusBlocked, TaskStatusCancelled:
		return true
	default:
		return false
	}
}
