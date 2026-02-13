---
id: 8
status: done
priority: High
blocked_by: []
tags: [sprint-1, epic, mvp]
---

# Sprint 1: MVP Core Loop

Deliver the MVP described in `MVP.md`: config -> SQLite -> scheduler enqueues -> queue dispatches -> plugin runs via protocol v1 -> state persists.

## Acceptance Criteria
- `ductile start --config <path>` runs in foreground and holds a single-instance lock.
- Scheduler tick loop enqueues `poll` jobs when due (with jitter) and prunes job log.
- Dispatch loop spawns plugin subprocess, performs protocol v1 request/response over stdin/stdout, and enforces timeouts.
- Plugin `state_updates` are shallow-merged into SQLite plugin state.
- Crash recovery behavior implemented per MVP/SPEC decision (recorded on the relevant card).
- Structured JSON logs emitted by core.

## Narrative
- 2026-02-08: Added explicit decision blocker for crash recovery policy (MVP vs SPEC mismatch) and a dedicated implementation card so this doesn't silently drift. (by @assistant)
- 2026-02-08: All Phase 2 components merged successfully (PRs #4, #5, #6): scheduler with jitter and crash recovery (Gemini), dispatch loop with timeouts (Claude), E2E echo plugin validation (Codex). Full test suite passing. Remaining work: wire components together in `cmd/ductile/main.go` to create runnable service with config loading, PID lock, queue initialization, scheduler start, and dispatch loop. (by @claude)
- 2026-02-09: **Sprint 1 MVP COMPLETE!** Phase 3 final integration finished (PR pending on `claude/main-cli`). All components wired in main.go. `ductile start` runs successfully: config loaded → PID lock acquired → database opened → plugins discovered → scheduler started (with crash recovery) → dispatcher started → echo plugin polls every 5m with jitter → state persists → graceful shutdown on Ctrl+C. All acceptance criteria met. Full test suite passing. Binary tested end-to-end. MVP is production-ready for personal-scale automation (< 50 jobs/day). Can be used for: data collection, health monitoring, data sync, personal automation, IoT integration, backup orchestration. Next: Sprint 2 (routing), Sprint 3 (webhooks), Sprint 4 (reliability controls). (by @claude)
