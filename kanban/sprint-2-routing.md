---
id: 21
status: backlog
priority: High
blocked_by: [47]
tags: [routing, epic, core]
---

# Event Routing & Plugin Chaining

Implement config-declared event routing: when a plugin emits events, the router matches them against configured routes and enqueues downstream `handle` jobs. This enables multi-hop workflows (e.g., webhook → transcript → extract → publish → notify).

## Job Story

When a plugin emits events during execution, I want the gateway to automatically route those events to downstream plugins based on config-declared rules, so I can build multi-step workflows without manual orchestration.

## Acceptance Criteria

- Parse `routes` array from config (from, event_type, to)
- Match plugin-emitted `events[].type` against routes
- Enqueue downstream `handle` jobs with event payload
- Fan-out: one event can trigger multiple downstream plugins
- Traceability: `parent_job_id` and `source_event_id` on downstream jobs
- Payload propagation strategy per RFC-005 decision (#47)
- Exact match only (no wildcards/regexes per SPEC)
- Preserve at-least-once semantics

## Dependencies

- **#47 (RFC-005 decision)** — Must decide payload propagation strategy before implementing
- Dispatch loop (exists) — needs to capture emitted events and pass to router
- Queue (exists) — router enqueues downstream jobs

## References

- SPEC.md §7 — Routing semantics
- RFC-005-ROUTING_OPTIONS.md — Payload flow strategies
- Card #47 — RFC-005 discussion (blocker)

## Narrative

