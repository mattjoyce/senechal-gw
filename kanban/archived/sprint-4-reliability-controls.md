---
id: 23
status: done
priority: Normal
blocked_by: [95, 96, 97]
tags: [sprint-4, epic, reliability]
---

# Sprint 4: Reliability Controls

Add the reliability features from SPEC: retries/backoff, deduplication, circuit breaker, and poll guard.

## Sub-Tasks
- #95 Implement Job Deduplication (Done)
- #96 Implement Retries & Exponential Backoff (Done)
- #97 Implement Circuit Breaker & Poll Guard (Done)

## Acceptance Criteria
- [x] Retry/backoff for retryable failures (and non-retryable conditions).
- [x] Dedupe via `dedupe_key` + `dedupe_ttl`.
- [x] Circuit breaker for scheduler-originated `poll` jobs.
- [x] Poll guard: no new `poll` job if one is already queued/running for the plugin (default 1).

## Narrative
- 2026-02-22: All sub-tasks (95, 96, 97) complete. Epic closed. (by @assistant)

