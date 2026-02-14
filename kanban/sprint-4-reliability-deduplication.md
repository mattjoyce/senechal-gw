---
id: 95
status: todo
priority: Normal
blocked_by: []
assignee: "@gemini"
tags: [sprint-4, reliability, deduplication, queue]
---

# Implement Job Deduplication

Enforce at-most-once semantics for specific tasks using `dedupe_key` and `dedupe_ttl`.

## Acceptance Criteria
- [ ] Enhance `Queue.Enqueue` to check for recent successful jobs with the same `dedupe_key`.
- [ ] Jobs matching an existing `succeeded` record within `dedupe_ttl` are not enqueued.
- [ ] Add `dedupe_ttl` configuration to `ServiceConfig` (default 24h).
- [ ] Log dropped jobs at `INFO` level with the `dedupe_key` and the ID of the existing successful job.
- [ ] Unit tests for deduplication hit/miss scenarios.

## Observability Requirements

For TUI watch (#TUI_WATCH_DESIGN.md) and operational diagnostics, this feature should emit:

**Events:**
```yaml
job.deduplicated:
  payload:
    job_id: string          # The duplicate that was dropped
    dedupe_key: string
    original_job_id: string # First job with same key
    ttl_remaining_seconds: int

job.dedupe_expired:
  payload:
    dedupe_key: string
    jobs_suppressed: int    # How many dupes were blocked during TTL
```

**Job metadata fields:**
- `dedupe_key` - the deduplication key (if any)
- `dedupe_ttl` - TTL duration
- `is_duplicate` - boolean flag indicating if this was a duplicate

**TUI usage:**
- Event stream: `15:23:45 job.deduplicated  backup-run [duplicate of 8a4c2d11]`
- Header panel (optional): `Deduped: 12 today` (stats)

## Narrative
- 2026-02-14: Created as a sub-task of epic #23. (by @gemini)
- 2026-02-15: Added observability requirements for TUI watch integration. (by @claude)
