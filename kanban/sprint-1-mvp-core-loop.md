---
id: 8
status: todo
priority: High
blocked_by: [9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 24, 25]
tags: [sprint-1, epic, mvp]
---

# Sprint 1: MVP Core Loop

Deliver the MVP described in `MVP.md`: config -> SQLite -> scheduler enqueues -> queue dispatches -> plugin runs via protocol v1 -> state persists.

## Acceptance Criteria
- `senechal-gw start --config <path>` runs in foreground and holds a single-instance lock.
- Scheduler tick loop enqueues `poll` jobs when due (with jitter) and prunes job log.
- Dispatch loop spawns plugin subprocess, performs protocol v1 request/response over stdin/stdout, and enforces timeouts.
- Plugin `state_updates` are shallow-merged into SQLite plugin state.
- Crash recovery behavior implemented per MVP/SPEC decision (recorded on the relevant card).
- Structured JSON logs emitted by core.

## Narrative
- 2026-02-08: Added explicit decision blocker for crash recovery policy (MVP vs SPEC mismatch) and a dedicated implementation card so this doesn’t silently drift. (by @assistant)
