---
id: 105
status: todo
priority: High
blocked_by: []
tags: [architecture, agent, external-service, protocol, api, scheduler]
---

# #105: External AgenticLoop Sibling (Ductile-Aware, API-Driven)

Build AgenticLoop as an external sibling service, not as an in-process Ductile plugin loop.

The sibling is long-lived and stateful. Ductile remains short-job, queue-based, serial-dispatch orchestration.

## Decision Summary

- Keep Ductile as a bounded execution engine.
- Move long-running cognition and iterative planning outside Ductile.
- Use Ductile API calls as tools (`/plugin/{plugin}/{command}` and optional `/pipeline/{name}`).
- Define wake-up success as **accepted**, not completed.

This avoids forcing Ductile to simulate daemon-like behavior inside a spawn-per-command model.

## Why This Exists

We learned from #103 and #104:

1. Resumable internal plugins can work, but they force additional routing/correlation complexity.
2. Serial dispatch is intentional for predictability; long-lived internal loops fight that model.
3. Scheduler needs an async wake-up capability that can conclude quickly without claiming task completion.

## Problem Statement

We need:

- Scheduled agent wake-ups managed by Ductile.
- Long-running goal execution without dispatcher lock contention.
- Strong auditability and deterministic correlation of actions/results.

We do not need:

- Ductile core concurrency redesign in MVP.
- Internal daemon/plugin lifecycle semantics.

## Proposed Architecture

Components:

1. **Ductile**:
   - Runs schedules.
   - Executes bounded jobs.
   - Optionally emits completion events from wake jobs.

2. **Wake Plugin (small, bounded)**:
   - Receives scheduled/event trigger.
   - Calls external AgenticLoop service `POST /v1/wake`.
   - Exits quickly with success if request is accepted.

3. **AgenticLoop Sibling Service (external)**:
   - Maintains run state and loop logic.
   - Calls Ductile APIs as tools.
   - Emits status/progress/completion independently (push and/or pull model).

## Contract: Wake vs Work

Two separate success states must be explicit:

1. **Wake Accepted** (Ductile job success):
   - Agent service received request and returned `accepted=true`.
   - Response includes `run_id`.

2. **Run Completed** (external run lifecycle):
   - Tracked out-of-band via status API and/or callback events.

Never conflate these in logs, metrics, or UI.

## API Contract (External Agent Service)

### `POST /v1/wake`

Purpose:
- Start or dedupe an agent run.

Request:
```json
{
  "wake_id": "stable-idempotency-key",
  "goal": "fetch https://example.com and produce a 2 para critique",
  "context": {
    "source": "schedule.daily",
    "requested_by": "ductile"
  },
  "constraints": {
    "max_loops": 10,
    "deadline_at": "ISO8601"
  }
}
```

Response (`202 Accepted`):
```json
{
  "accepted": true,
  "run_id": "run_01...",
  "status_url": "/v1/runs/run_01..."
}
```

Rules:
- Must be idempotent on `wake_id`.
- Duplicate wake must return same `run_id`.

### `GET /v1/runs/{run_id}`

Purpose:
- Poll run status.

Response:
```json
{
  "run_id": "run_01...",
  "state": "queued|running|done|failed|cancelled",
  "started_at": "ISO8601",
  "updated_at": "ISO8601",
  "summary": "short text",
  "steps": [
    {"step": 1, "tool": "jina-reader", "status": "ok"}
  ]
}
```

### Optional callback/webhook from sibling -> Ductile

Use when push updates are preferred over polling:
- `agent.progress`
- `agent.completed`
- `agent.failed`

These are run-lifecycle notifications, not wake acknowledgements.

## Ductile Integration Contract

Wake plugin responsibilities:

1. Validate required fields (`goal`, optional `context`, optional `wake_id`).
2. `POST /v1/wake` with timeout <= plugin timeout budget.
3. Return Ductile protocol response:
   - `status: ok` if accepted.
   - `state_updates.last_run_id`.
   - optional event `agent.wake.accepted`.
4. Return `status: error` only on hard wake failure (network/auth/4xx/5xx).

Tool invocation from sibling:

1. Prefer `POST /plugin/{plugin}/{command}` for direct calls.
2. Use strict allowlist in sibling config.
3. Include correlation headers/fields:
   - `run_id`, `step`, `wake_id`.

## Security Requirements

1. Separate tokens:
   - Ductile -> sibling wake token.
   - Sibling -> Ductile tool token.
2. Sibling tool token must be least-privilege:
   - no `admin:*`
   - command-level/plugin-level allowlist.
3. Explicit deny for dangerous commands by default.
4. Request signing or mTLS if crossing trust boundaries.

## Reliability & Idempotency

Mandatory:

1. Idempotent wake (`wake_id`).
2. Idempotent tool step execution key:
   - `run_id:step:tool:attempt`.
3. Durable run state store (SQLite/Postgres/Redis+snapshot).
4. Crash-safe resume on sibling restart.
5. Clear retry policy with max attempts per step and per run.

## Observability

Emit structured logs with:

- `run_id`
- `wake_id`
- `step`
- `tool`
- `state_transition`
- `latency_ms`
- `error_class`

Metrics:

- wake acceptance latency
- run duration
- runs by terminal status
- tool call success/failure rate
- retries and dedupe hits

## Non-Goals (Now)

1. Ductile multi-worker dispatch redesign.
2. Wildcard router semantics in Ductile.
3. Internal long-running plugin daemons.
4. Merging #103/#104 branches.

## Reuse From #103 / #104

Keep and reuse now:

1. Correlation invariant: `run_id + step + tool`.
2. Stale/duplicate handling and escalation semantics.
3. Workspace paper trail conventions.
4. Safety budget concept (`max_loops`, escalation on exhaustion).

Do not reuse now:

1. Internal resumable plugin routing complexity.
2. Per-tool pipeline boilerplate as primary mechanism.

## Assumptions To Challenge (Required Review Checklist)

Before implementation, verify each assumption:

1. Assumption: wake acceptance is enough for scheduler success.
   - Challenge: do consumers mistakenly treat this as completed work?
2. Assumption: sibling service uptime is high enough for scheduling SLA.
   - Challenge: what is fallback behavior when sibling is down?
3. Assumption: tool APIs are fast enough for iterative loops.
   - Challenge: do slow tools starve sibling worker capacity?
4. Assumption: retries are safe for all called commands.
   - Challenge: are write commands truly idempotent?
5. Assumption: one sibling instance is sufficient.
   - Challenge: what is shard/HA plan when run volume grows?

## Implementation Plan (for less-capable coding agents)

Phase 1: Contract and skeleton
1. Implement external service with `POST /v1/wake`, `GET /v1/runs/{id}`.
2. Add durable run store and idempotency table.
3. Add Ductile wake plugin that only submits wake requests.

Phase 2: Tool execution loop
1. Implement Frame->Plan->Act->Reflect loop in sibling.
2. Implement Ductile API client with strict allowlist.
3. Persist every state transition and tool result.

Phase 3: Reliability and hardening
1. Add retries/backoff with per-step caps.
2. Add cancellation/deadline enforcement.
3. Add structured logs and metrics.

Phase 4: Operator UX
1. Add run inspect endpoint/output.
2. Add progress callbacks into Ductile (optional).
3. Add runbook for recovery and replay.

## Acceptance Criteria

- [ ] Ductile scheduled wake job succeeds on `accepted=true` only.
- [ ] Sibling returns deterministic `run_id` for duplicate `wake_id`.
- [ ] Sibling can execute at least two Ductile tool calls in one run.
- [ ] Run status is queryable independently of wake job status.
- [ ] Token scopes enforce least privilege for sibling tool calls.
- [ ] Restarting sibling does not lose active run progress.
- [ ] Clear docs distinguish wake acceptance from run completion.
- [ ] #103 and #104 branches remain unmerged.

## Narrative

- 2026-02-16: Created card #105 to replace internal AgenticLoop plugin direction with an external Ductile-aware sibling service. Captured decisions, API contract, reliability/security requirements, and phased implementation guidance. (by Codex)
