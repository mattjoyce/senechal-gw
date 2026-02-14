---
id: 96
status: done
priority: High
blocked_by: []
assignee: "@gemini"
tags: [sprint-4, reliability, retry, backoff, dispatcher]
---

# Implement Retries & Exponential Backoff

Automate recovery for transient job failures using exponential backoff delays.

## Acceptance Criteria
- [x] Update `Dispatcher` to re-enqueue failed/timed-out jobs if `attempt < max_attempts`.
- [x] Implement exponential backoff: `delay = base * 2^(attempt-1) + jitter`.
- [x] Support non-retryable failures:
    - [x] Plugin exit code `78` (Configuration error).
    - [x] Plugin response `"retry": false`.
- [x] Update SQL `Dequeue` logic to strictly respect `next_retry_at`.
- [x] Unit tests for backoff calculation and retry state transitions.

## Observability Requirements

For TUI watch (#TUI_WATCH_DESIGN.md) and operational diagnostics, this feature should emit:

**Events:**
```yaml
job.retry_scheduled:
  payload:
    job_id: string
    attempt: int        # Current attempt number (1, 2, 3...)
    max_attempts: int   # Configured max
    backoff_ms: int     # Delay before next attempt
    next_retry_at: timestamp
    reason: string      # Why it failed ("timeout", "plugin_error", etc.)

job.retry_exhausted:
  payload:
    job_id: string
    attempts: int       # How many we tried
    final_error: string
```

**Job metadata fields:**
- `retry_count` - current attempt number
- `max_retries` - from config
- `next_retry_at` - timestamp for next attempt
- `backoff_schedule` - array of delays used

**TUI usage:** Pipelines panel shows `◉ Job abc123 (attempt 2/3, retry in 4s)`

## Narrative
- 2026-02-14: Created as a sub-task of epic #23. (by @gemini)
- 2026-02-15: Added observability requirements for TUI watch integration. (by @claude)
- 2026-02-15: Implementation started on branch `card-96-retries-backoff`; adding dispatcher retry scheduling with exponential backoff + jitter, non-retryable handling (`retry:false`, exit code 78), and retry observability events. (by @codex)
- 2026-02-15: Completed dispatcher retry scheduling for failed/timed-out jobs using exponential backoff (`base * 2^(attempt-1) + jitter`) and `next_retry_at` requeue semantics. Added non-retryable handling for plugin exit code `78` and protocol response `retry:false`. Emitted `job.retry_scheduled` and `job.retry_exhausted` events with retry/backoff metadata for TUI watch. Added unit/integration coverage for backoff math, retry scheduling, non-retryable transitions, and `next_retry_at` dequeue gating. Verified with `go test ./internal/queue ./internal/dispatch ./internal/e2e -count=1`. (by @codex)
