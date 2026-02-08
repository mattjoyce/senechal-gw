---
id: 21
status: backlog
priority: Normal
blocked_by: []
tags: [sprint-2, epic, routing]
---

# Sprint 2: Routing

Implement config-declared event routing and downstream job creation (plugin chaining).

## Acceptance Criteria
- Parse `routes` from `config.yaml`.
- Match plugin-emitted `events[].type` against routes and enqueue downstream `handle` jobs.
- Inject traceability fields (`parent_job_id`, `source_event_id`) per SPEC.
- Preserve at-least-once semantics; downstream jobs inherit `dedupe_key` when present.

## Narrative

