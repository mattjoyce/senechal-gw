---
id: 15
status: todo
priority: High
blocked_by: [9, 10, 14]
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

