---
id: 96
status: todo
priority: High
blocked_by: []
assignee: "@gemini"
tags: [sprint-4, reliability, retry, backoff, dispatcher]
---

# Implement Retries & Exponential Backoff

Automate recovery for transient job failures using exponential backoff delays.

## Acceptance Criteria
- [ ] Update `Dispatcher` to re-enqueue failed/timed-out jobs if `attempt < max_attempts`.
- [ ] Implement exponential backoff: `delay = base * 2^(attempt-1) + jitter`.
- [ ] Support non-retryable failures:
    - [ ] Plugin exit code `78` (Configuration error).
    - [ ] Plugin response `"retry": false`.
- [ ] Update SQL `Dequeue` logic to strictly respect `next_retry_at`.
- [ ] Unit tests for backoff calculation and retry state transitions.

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
