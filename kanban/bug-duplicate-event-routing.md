---
id: 60
status: todo
priority: High
blocked_by: [52]
assignee: "@gemini"
tags: [bug, orchestration, reliability]
---

# Bug: Parent retry causes duplicate event routing

When a job is retried (e.g. after a crash or timeout), it re-executes and emits the same events. The Dispatcher's `routeEvents` currently enqueues new child jobs for every event emitted, even if they were already enqueued in a previous attempt. This leads to an exponential explosion of jobs.

## Acceptance Criteria
- `Dispatcher.routeEvents` must be idempotent across parent job retries.
- Use `source_event_id` and `dedupe_key` to prevent re-enqueuing already triggered events.
- Add an E2E test that simulates a parent job retry and verifies no duplicate child jobs are created.

## Narrative
- 2026-02-12: Identified during review of Sprint 4 integration logic. (by @gemini)
