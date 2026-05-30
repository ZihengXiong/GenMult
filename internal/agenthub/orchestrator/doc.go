// Package orchestrator implements AgentHub's task orchestration control plane.
//
// Scope:
//   - plan a user objective into a task DAG
//   - move runs/tasks through hard-checked state machines
//   - dispatch ready tasks to AgentProvider adapters
//   - collect attempts and append replayable events
//   - reconcile active runs after crashes or timeouts
//
// The package intentionally does not expose HTTP handlers or depend on sqlc.
// API/database layers should implement Store and call Service.StartRun,
// Service.ReconcileRun, Service.ReconcileActiveRuns, Service.CancelRun, and
// Store.ListEvents for frontend projection.
package orchestrator
