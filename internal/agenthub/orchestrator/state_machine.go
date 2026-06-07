package orchestrator

func canTransitionRun(from, to RunStatus) bool {
	if from == to {
		return true
	}
	if isTerminalRunStatus(from) {
		return false
	}
	switch from {
	case "":
		return to == RunStatusDraft || to == RunStatusPlanning
	case RunStatusDraft:
		return to == RunStatusPlanning || to == RunStatusCancelled
	case RunStatusPlanning:
		return to == RunStatusDispatching || to == RunStatusFailed || to == RunStatusCancelled
	case RunStatusDispatching:
		return to == RunStatusCollecting || to == RunStatusCompleted || to == RunStatusFailed || to == RunStatusCancelled
	case RunStatusCollecting:
		return to == RunStatusDispatching || to == RunStatusCompleted || to == RunStatusFailed || to == RunStatusCancelled
	default:
		return false
	}
}

func canTransitionTask(from, to TaskStatus) bool {
	if from == to {
		return true
	}
	if isTerminalTaskStatus(from) {
		// failed can be retried by moving it back to ready when the retry budget allows.
		if from == TaskStatusFailed && to == TaskStatusReady {
			return true
		}
		// blocked can be unblocked when upstream dependencies are resolved.
		if from == TaskStatusBlocked && (to == TaskStatusReady || to == TaskStatusPending) {
			return true
		}
		return false
	}
	switch from {
	case "":
		return to == TaskStatusPending || to == TaskStatusReady
	case TaskStatusPending:
		return to == TaskStatusReady || to == TaskStatusBlocked || to == TaskStatusCancelled
	case TaskStatusReady:
		return to == TaskStatusRunning || to == TaskStatusCancelled
	case TaskStatusRunning:
		return to == TaskStatusSucceeded || to == TaskStatusFailed || to == TaskStatusCancelled
	default:
		return false
	}
}
