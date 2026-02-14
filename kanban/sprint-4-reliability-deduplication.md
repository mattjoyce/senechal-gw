---
id: 95
status: done
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
- 2026-02-14: Implementation started on branch `card-95-deduplication`; plan is queue-level dedupe check with TTL, INFO drop logging, scheduler handling for dedupe drop, and queue tests for hit/miss semantics. (by @codex)
- 2026-02-14: Completed queue-level dedupe enforcement using `dedupe_key` + `service.dedupe_ttl` window against recent `succeeded` jobs. Duplicate enqueues now drop with INFO logging (`dedupe_key`, existing job id) and return a typed dedupe-drop error. Scheduler poll path now treats dedupe-drop as expected behavior (no error escalation). Added queue tests for miss-before-success, hit-after-success, and TTL-expiry re-allow scenarios; verified `go test ./internal/queue` and `go test ./cmd/ductile`. (by @codex)
