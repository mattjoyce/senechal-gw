---
id: 16
status: todo
priority: High
blocked_by: [9, 12, 13, 14, 17, 19]
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

