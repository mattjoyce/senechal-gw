---
id: 97
status: done
priority: Normal
blocked_by: []
assignee: "@gemini"
tags: [sprint-4, reliability, circuit-breaker, poll-guard, scheduler]
---

# Implement Circuit Breaker & Poll Guard

Protect external systems and infrastructure by failing fast when plugins are unhealthy or overloaded.

## Acceptance Criteria
- [x] Create `circuit_breakers` SQLite table to track failure counts and cooldowns.
- [x] Implement Circuit Breaker logic in `Scheduler`:
    - [x] Stop enqueuing `poll` jobs if `failure_count >= threshold`.
    - [x] Circuit resets after `reset_after` duration or manual reset via CLI.
- [x] Implement Poll Guard:
    - [x] Check for existing `queued` or `running` poll jobs before enqueuing.
    - [x] Respect `max_outstanding_polls` (default 1).
- [x] Unit tests for state transitions (Closed -> Open -> Half-Open/Closed).

## Observability Requirements

For TUI watch (#TUI_WATCH_DESIGN.md) and operational diagnostics, this feature should emit:

**Events:**
```yaml
circuit.opened:
  payload:
    plugin: string
    endpoint: string    # Which command/endpoint
    failure_count: int
    threshold: int
    cooldown_seconds: int

circuit.half_open:
  payload:
    plugin: string
    endpoint: string
    test_job_id: string # Job attempting recovery

circuit.closed:
  payload:
    plugin: string
    endpoint: string
    recovery_time_seconds: int

poll.throttled:
  payload:
    plugin: string
    reason: string      # "circuit_open" or "rate_limit"
    next_poll_at: timestamp
```

**Health endpoint additions:**
```json
{
  "circuits": {
    "fabric": {"state": "open", "failures": 5, "since": "2024-02-15T10:23:00Z"},
    "echo": {"state": "closed", "failures": 0}
  }
}
```

**TUI usage:**
- Header panel: `⚠️ 1 circuit open` (warning indicator)
- Scheduler panel: `fabric/poll - ⊘ Circuit open, retry in 5m`
- Event stream: Show circuit state transitions

## Narrative
- 2026-02-14: Created as a sub-task of epic #23. (by @gemini)
- 2026-02-14: Started implementation on branch `card-97-circuit-breaker-poll-guard`; adding persistent circuit breaker state, scheduler poll guard enforcement, and CLI reset support. (by @codex)
- 2026-02-14: Completed circuit breaker + poll guard reliability controls. Added persistent `circuit_breakers` table and queue APIs for state/get/reset, latest scheduler poll result reconciliation, and outstanding poll counting. Scheduler now gates poll enqueue on circuit state (`closed/open/half_open`) and `max_outstanding_polls`, and supports cooldown-based half-open probing. Added manual reset command `ductile system reset <plugin> [--config PATH]` (plus root alias `ductile reset`). Verified with `go test ./internal/storage ./internal/queue ./internal/scheduler ./cmd/ductile -count=1`. (by @codex)
- 2026-02-15: Added observability requirements for TUI watch integration. (by @claude)
