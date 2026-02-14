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

## Narrative
- 2026-02-14: Created as a sub-task of epic #23. (by @gemini)
