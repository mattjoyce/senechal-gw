---
id: 15
status: done
priority: High
blocked_by: []
tags: [sprint-1, mvp, scheduler]
---

# Scheduler Tick Loop + Fuzzy Intervals

Implement the heartbeat scheduler that checks due plugins and enqueues `poll` jobs using jittered intervals.

## Acceptance Criteria
- Tick loop runs at `service.tick_interval` (default 60s).
- Computes `next_run` using SPEC jitter semantics (per scheduled run, fixed jitter).
- Enqueues a `poll` job when due for the enabled plugin.
- Prunes completed job log on each tick if `job_log` exists (per MVP).

## Narrative
- 2026-02-08: Implemented scheduler tick loop in `internal/scheduler/scheduler.go` with `calculateJitteredInterval()` for randomized scheduling, `parseScheduleEvery()` supporting named intervals (5m, hourly, daily, etc.), and `enqueuePollJob()` with deduplication keys. Tick loop enqueues poll jobs for enabled plugins with jitter and prunes job logs based on retention policy. Comprehensive table-driven tests cover jitter calculation, interval parsing, and tick behavior. Merged via PR #6. (by @gemini)
