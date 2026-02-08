---
id: 14
status: todo
priority: High
blocked_by: [9, 11]
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

