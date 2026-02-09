---
id: 14
status: done
priority: High
blocked_by: []
tags: [sprint-1, mvp, queue]
---

# SQLite Work Queue

Implement a SQLite-backed work queue so jobs survive process restarts and can be recovered safely.

## Acceptance Criteria
- Enqueue inserts a `queued` job row.
- Dequeue selects oldest `queued` job and marks it `running` atomically.
- Complete marks job `succeeded`/`failed` and persists relevant fields (`last_error`, timestamps).
- MVP behavior: failures are recorded; retry/backoff is out of scope unless explicitly pulled in.

## Narrative
- 2026-02-08: Implemented SQLite work queue in `internal/queue/queue.go` with `Enqueue()` (inserts queued jobs with UUID, deduplication support), `Dequeue()` (atomic FIFO selection using `ORDER BY id` with status transition to running), `Complete()` (marks succeeded/failed with timestamps and error messages), and helper methods for crash recovery (`FindJobsByStatus`, `UpdateJobForRecovery`). Includes `PruneJobLogs()` for retention management. FIFO ordering and atomicity verified in tests. Merged via PR #1. (by @codex)
