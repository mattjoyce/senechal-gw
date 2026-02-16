---
id: 103
status: backlog
priority: High
blocked_by: []
tags: [architecture, rfc, plugin, agentic, llm, skills]
---

# RFC-103: Resumable AgenticLoop Plugin (No Dispatcher Changes)

## Goal

Implement an `agentic-loop` plugin that can complete multi-step, goal-directed tasks (Frame -> Plan -> Act -> Reflect) **without** requiring concurrent dispatch, nested dispatch, or special exit codes.

This RFC is written as implementation instructions for a developer with no prior project context.

## Why This Design

Current core behavior is serial FIFO dispatch (one running job at a time).  
A long-running agent that synchronously calls tools can deadlock because child jobs cannot run while the parent agent job is still running.

This RFC avoids that by making the agent **resumable**:
- One agent turn per job.
- Agent exits normally.
- Tool result returns as a routed event.
- A new agent job resumes using persisted state.

## Non-Goals

- No special process exit codes (for example `98`/`99`) to control orchestration.
- No nested synchronous dispatch.
- No multi-queue or worker-pool redesign in this RFC.
- No recursive agent spawning in MVP.

## Core Invariants

1. Every run has a unique `run_id`.
2. Every tool action has a monotonically increasing `step` integer (starting at 1).
3. Agent must persist `pending_step` and `pending_tool` before dispatching a tool.
4. Tool result events must include `run_id`, `step`, and `tool`.
5. Agent resumes only when `(run_id, step, tool)` matches current pending state.
6. Duplicate/stale events are ignored safely.
7. All plugin processes exit with normal protocol v2 success/error semantics.

## Runtime Model (Single Turn)

Each `agentic-loop` `handle` invocation performs exactly one turn:

1. Load run state from plugin `state` and workspace files.
2. If inbound event is `agentic.start`: initialize new run.
3. If inbound event is `agentic.tool_result`: validate correlation against pending state.
4. Run LLM logic for needed phases in this invocation:
- First turn: Frame + Plan + Act(dispatch only).
- Resume turn: Reflect + Plan + Act(dispatch only) OR Reflect + Done.
5. Persist state updates.
6. Emit events (tool request or final completion).
7. Return protocol v2 response and exit.

## Event Contract

### 1) Start Event (external -> agent)

`type: agentic.start`

```json
{
  "type": "agentic.start",
  "payload": {
    "goal": "fetch web http://mattjoyce.ai and produce a 2 para critique",
    "context": {
      "tone": "constructive",
      "max_steps": 6
    }
  },
  "dedupe_key": "agentic:start:<external-request-id>"
}
```

### 2) Tool Request Event (agent -> tool plugin route target)

`type: agentic.tool_request`

```json
{
  "type": "agentic.tool_request",
  "payload": {
    "run_id": "01JABC...",
    "step": 1,
    "tool": "fetch",
    "tool_command": "handle",
    "tool_payload": {
      "url": "http://mattjoyce.ai"
    },
    "requested_at": "2026-02-16T12:00:00Z"
  },
  "dedupe_key": "agentic:run:01JABC:step:1:request"
}
```

### 3) Tool Result Event (tool -> agent)

`type: agentic.tool_result`

```json
{
  "type": "agentic.tool_result",
  "payload": {
    "run_id": "01JABC...",
    "step": 1,
    "tool": "fetch",
    "status": "ok",
    "result": {
      "artifact_path": "artifacts/fetch-01.html",
      "content_type": "text/html",
      "excerpt": "..."
    },
    "error": null,
    "completed_at": "2026-02-16T12:00:05Z"
  },
  "dedupe_key": "agentic:run:01JABC:step:1:result"
}
```

### 4) Completion Event (agent -> downstream)

`type: agent.completed`

```json
{
  "type": "agent.completed",
  "payload": {
    "run_id": "01JABC...",
    "goal": "fetch web http://mattjoyce.ai and produce a 2 para critique",
    "outcome": "Two paragraph critique generated in artifacts/critique.md",
    "steps_taken": 2,
    "artifacts": ["artifacts/critique.md"]
  }
}
```

## Required Router Configuration

This RFC requires explicit routes; no hidden orchestration.

```yaml
routes:
  - from: agentic-loop
    event_type: agentic.tool_request
    to: tool-router

  - from: tool-router
    event_type: agentic.tool_result
    to: agentic-loop
```

`tool-router` is a simple plugin that reads `payload.tool` and emits a trigger event to the named tool plugin/command.  
Alternative: replace `tool-router` with static routes if tool set is fixed.

## Agent Plugin State Schema

Persist in plugin `state` (SQLite JSON blob):

```json
{
  "runs": {
    "01JABC...": {
      "status": "running",
      "goal": "...",
      "created_at": "ISO8601",
      "updated_at": "ISO8601",
      "step": 1,
      "max_steps": 6,
      "reframes": 0,
      "max_reframes": 2,
      "pending_step": 1,
      "pending_tool": "fetch",
      "pending_since": "ISO8601",
      "workspace_ref": "job:<root_job_id>"
    }
  },
  "last_run_id": "01JABC..."
}
```

State rules:
- `pending_step`/`pending_tool` are required before emitting `agentic.tool_request`.
- When matching `agentic.tool_result` arrives, clear pending fields.
- On completion or escalation, set run status terminal (`done` or `escalated`).

## Workspace Contract

Within each run workspace:

```text
workspace/<job-id>/
  context.md
  memory.md
  plan.md
  trace.jsonl
  artifacts/
```

Rules:
- Agent writes only append/update-safe files.
- Tool outputs should be artifact files when payload is large.
- Tool result event should carry artifact path, not full large content.

## Protocol Behavior (No Ambiguity)

- Agent process must return exit code `0` for successful turn completion.
- Agent process must return protocol response with:
  - `status: ok` when turn succeeded (even if run is still in progress).
  - `events` containing either `agentic.tool_request` or `agent.completed`.
  - `state_updates` with persisted run state changes.
- Non-zero process exit is treated as failure by existing retry logic; do not use for control flow.

## Resume Validation Logic

When processing `agentic.tool_result`, agent must validate in this order:

1. `run_id` exists in state. If not, ignore and log warn.
2. Run status is `running`. If terminal, ignore and log info.
3. `step == pending_step`. If lower, ignore as stale/duplicate.
4. `tool == pending_tool`. If mismatch, escalate run as protocol violation.
5. `status`:
- `ok`: proceed to Reflect.
- `error`: decide retry/escalate based on policy and step budget.

## Worked Example (Requested Scenario)

User asks: "fetch web http://mattjoyce.ai and produce a 2 para critique."

1. External caller emits `agentic.start` with goal text.
2. Agent starts run `run_id=R1`, writes context/memory/plan.
3. Agent decides step 1: call `fetch.handle(url=http://mattjoyce.ai)`.
4. Agent saves pending:
- `pending_step=1`
- `pending_tool=fetch`
5. Agent emits `agentic.tool_request` and exits `status: ok`.
6. Router sends request to fetch tool path.
7. Fetch plugin writes HTML artifact and emits `agentic.tool_result` with `run_id=R1, step=1, tool=fetch`.
8. Router sends `agentic.tool_result` to `agentic-loop`.
9. Agent resumes R1, validates correlation, reflects, plans step 2: generate critique.
10. Agent dispatches step 2 tool request (for example `fabric.handle` with prompt + artifact path), exits `status: ok`.
11. Result returns as `agentic.tool_result` step 2.
12. Agent resumes, Reflect decides `done`, emits `agent.completed` with artifact `artifacts/critique.md`, exits `status: ok`.

At no point is there a special exit code.

## Idempotency and Dedupe

- Agent tool-request events use dedupe key:
  - `agentic:run:<run_id>:step:<step>:request`
- Tool-result events use dedupe key:
  - `agentic:run:<run_id>:step:<step>:result`
- Agent should additionally maintain `completed_steps` set per run in state to safely ignore duplicates after retries.

## Error Handling

- Unknown `run_id`: ignore + log warn.
- Mismatched `pending_tool`: mark run `escalated` and emit `agent.escalated`.
- Tool error with retries remaining: re-issue same step request with same `step` and increment `attempt` counter in state.
- Tool error with no retries remaining: `agent.escalated`.
- Budget exceeded (`step >= max_steps`): `agent.escalated`.
- Reframe count exceeded: `agent.escalated`.

## Security and Scope

- Agent API token must not include `admin:*`.
- Agent must not be allowed to trigger `agentic-loop` directly.
- Tool allowlist enforced in agent config:

```yaml
plugins:
  agentic-loop:
    schedule: null
    timeouts:
      handle: 120s
    config:
      max_steps: 20
      max_reframes: 2
      allowed_plugins: [fetch, fabric, jina-reader]
```

## Implementation Plan (Developer Checklist)

1. Create plugin `plugins/agentic-loop/` with protocol v2 `handle`.
2. Implement run state load/save (`run_id`, step, pending fields).
3. Implement `agentic.start` path (init workspace files + first plan).
4. Implement `agentic.tool_result` path (validate -> reflect -> next decision).
5. Emit `agentic.tool_request` event for Act.
6. Emit `agent.completed` on done; `agent.escalated` on terminal error.
7. Add/update router path to feed `agentic.tool_result` back to `agentic-loop`.
8. Add table-driven tests for correlation, dedupe, stale events, and budget limits.

## Test Cases (Must Pass)

- Start event creates run and first tool request.
- Matching tool result resumes and advances to next step.
- Duplicate tool result does not double-advance step.
- Stale step result is ignored.
- Wrong tool for pending step escalates.
- `max_steps` cutoff escalates deterministically.
- Completion emits exactly one `agent.completed`.

## Success Criteria

- [ ] Agent loop works end-to-end with serial dispatcher unchanged.
- [ ] No synchronous nested dispatch used.
- [ ] No special exit code used for control flow.
- [ ] Tool-to-agent resume is deterministic via `run_id + step + tool`.
- [ ] Duplicate/stale events are handled safely.
- [ ] Final artifacts are available in workspace and referenced in completion event.

## References

- `docs/ARCHITECTURE.md` (Queue semantics, protocol v2, routes, API)
- RFC-92: Pipeline Execution Modes
- RFC-004: LLM as Operator/Admin

## Narrative

- 2026-02-16: Initial RFC created. AgenticLoop as a long-running plugin using existing sync API infrastructure to invoke other plugins as tools. (by @claude)
- 2026-02-16: Review comments appended. (by Codex)
- 2026-02-16: Replaced long-running sync design with explicit resumable multi-job state machine (`run_id`, `pending_step`, `pending_tool`) and concrete event/routing contracts so implementation is unambiguous. (by Codex)
