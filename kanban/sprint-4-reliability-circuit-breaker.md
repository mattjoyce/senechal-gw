---
id: 97
status: todo
priority: Normal
blocked_by: []
assignee: "@gemini"
tags: [sprint-4, reliability, circuit-breaker, poll-guard, scheduler]
---

# Implement Circuit Breaker & Poll Guard

Protect external systems and infrastructure by failing fast when plugins are unhealthy or overloaded.

## Acceptance Criteria
- [ ] Create `circuit_breakers` SQLite table to track failure counts and cooldowns.
- [ ] Implement Circuit Breaker logic in `Scheduler`:
    - [ ] Stop enqueuing `poll` jobs if `failure_count >= threshold`.
    - [ ] Circuit resets after `reset_after` duration or manual reset via CLI.
- [ ] Implement Poll Guard:
    - [ ] Check for existing `queued` or `running` poll jobs before enqueuing.
    - [ ] Respect `max_outstanding_polls` (default 1).
- [ ] Unit tests for state transitions (Closed -> Open -> Half-Open/Closed).

## Narrative
- 2026-02-14: Created as a sub-task of epic #23. (by @gemini)
