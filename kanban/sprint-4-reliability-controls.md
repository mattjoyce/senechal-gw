---
id: 23
status: backlog
priority: Normal
blocked_by: []
tags: [sprint-4, epic, reliability]
---

# Sprint 4: Reliability Controls

Add the reliability features from SPEC: retries/backoff, deduplication, circuit breaker, and poll guard.

## Acceptance Criteria
- Retry/backoff for retryable failures (and non-retryable conditions).
- Dedupe via `dedupe_key` + `dedupe_ttl`.
- Circuit breaker for scheduler-originated `poll` jobs.
- Poll guard: no new `poll` job if one is already queued/running for the plugin (default 1).

## Narrative

