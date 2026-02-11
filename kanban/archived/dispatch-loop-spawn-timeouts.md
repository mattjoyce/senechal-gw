---
id: 16
status: done
priority: High
blocked_by: []
tags: [sprint-1, mvp, dispatch]
---

# Dispatch Loop: Spawn Plugin + Enforce Timeouts

Pull jobs from the queue and execute them via a spawn-per-command subprocess using protocol v1.

## Acceptance Criteria
- Spawns `<plugins_dir>/<plugin>/<entrypoint>` for the job's command.
- Writes request envelope to stdin, closes stdin, reads stdout until EOF or timeout.
- Enforces timeout: SIGTERM, 5s grace, then SIGKILL (per SPEC).
- Captures stderr (capped) and records it for debugging.
- On success: applies `state_updates` and marks job `succeeded`.
- On failure/protocol error: marks job `failed` and stores `last_error`.
- Ignores `events` in MVP (routing out of scope).

## Narrative
- 2026-02-08: Implemented dispatcher in `internal/dispatch/dispatcher.go` with `ExecuteJob()` handling plugin subprocess lifecycle: spawns plugin entrypoint, encodes protocol v1 request to stdin, reads response from stdout with context-based timeout, enforces SIGTERM → 5s grace → SIGKILL sequence, captures stderr (capped at 4KB), applies state updates via shallow merge, and records job completion with logs. Supports per-command timeout overrides from config. Comprehensive tests cover success, error, timeout, and protocol error paths using test helper scripts. Merged via PR #4. (by @claude)
