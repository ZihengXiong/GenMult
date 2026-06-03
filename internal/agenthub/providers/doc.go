// Package providers implements AgentProvider adapters for external CLI-based
// coding agents (Claude Code, OpenAI Codex).
//
// Scope:
//   - spawn CLI subprocesses with context propagation and timeout.
//   - parse NDJSON/JSONL stdout streams into provider-agnostic events.
//   - resolve workspace working directories via WorkspaceResolver.
//   - forward intermediate events to the orchestrator event store.
//   - classify errors into retryable vs terminal categories.
package providers
