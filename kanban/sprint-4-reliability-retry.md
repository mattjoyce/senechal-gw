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

## Narrative
- 2026-02-14: Created as a sub-task of epic #23. (by @gemini)
