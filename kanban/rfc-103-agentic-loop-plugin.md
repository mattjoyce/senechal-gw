---
id: 103
status: backlog
priority: High
blocked_by: []
tags: [architecture, rfc, plugin, agentic, llm, skills]
---

# RFC-95: AgenticLoop Plugin

## Problem Statement

Ductile can orchestrate linear and branching pipelines, but has no way to express **goal-directed, iterative work** — tasks where the path to completion isn't known upfront. Examples: "summarise this paper and find three related works", "research X and write a report", "monitor this feed and alert me when Y happens". These require a cognitive loop: assess the goal, plan a step, execute it, evaluate progress, repeat.

## Proposal

A new plugin called `agentic-loop` that implements a Frame→Plan→Act→Reflect loop as a **single long-running plugin process**. It uses workspace files as its working memory and invokes other Ductile plugins as tools via the existing synchronous API.

No core changes required. The AgenticLoop is just a plugin that happens to be an API client of its own gateway.

## Triggering

All normal trigger methods work:

- **Webhook** — external system posts a goal
- **API** — `POST /api/trigger/agentic-loop` with goal in payload
- **Schedule** — periodic goal execution (e.g., daily digest)
- **Route** — another plugin emits an event that triggers the loop

The inbound event payload must include a `goal` field and may include optional `context` (background info, constraints, preferences).

## Workspace Layout

The core spawns the plugin with a `workspace_dir` as usual. AgenticLoop initialises it on startup:

```
workspace/<job-id>/
├── context.md       # Seeded from inbound event payload (goal + background)
├── memory.md        # Working memory — written by Frame, updated every phase
├── skills.md        # Auto-generated from plugin manifests (read-only)
├── plan.md          # Current todo list — written by Plan, updated by Reflect
├── artifacts/       # Files produced during Act phases
└── trace.jsonl      # Append-only structured execution log
```

### skills.md Generation

`skills.md` must be seeded into the workspace before the loop starts. Two options:

**A. Core seeds it (preferred):** The dispatcher generates `skills.md` from the plugin registry and writes it to `workspace_dir` before spawning AgenticLoop. This keeps the plugin simple and the skill catalog always current.

**B. Plugin generates it:** AgenticLoop calls a hypothetical `/api/plugins` endpoint to discover available skills. More self-contained but adds an API dependency at startup.

Content of `skills.md` — for each available plugin:
- Plugin name and description
- Available commands (poll, handle, health)
- Config keys and descriptions
- Example input/output (if declared in manifest)

## The Loop

### 1. Start

- Receive `handle` command via protocol v2
- Read `event.payload.goal` and `event.payload.context`
- Write `context.md` from the inbound payload
- Initialise empty `memory.md`, `plan.md`, `trace.jsonl`
- Confirm `skills.md` is present (seeded by core or self-generated)
- Enter Frame

### 2. Frame

**Reads:** `context.md`, `memory.md`
**Purpose:** Turn the inbound task into a bounded objective.

**Writes to `memory.md`:**
- Goal statement (one sentence)
- Definition of done (3–7 checkboxes)
- Constraints (time/step budget, forbidden tools, privacy/security)
- Assumptions / unknowns (what's missing; whether we can proceed)
- Initial hypothesis (optional: what we think will work)

**Design note:** Frame can be re-entered from Reflect ("Reframe") if the original goal was misframed. Reframe appends a reframe record to `trace.jsonl` explaining why.

### 3. Plan

**Reads:** `skills.md`, `memory.md`, `plan.md`
**Purpose:** Decide the next step and tool usage.

**Writes to `plan.md`:**
- Step list / outline (kept short; refined each iteration)
- Next action (exactly one tool call or dispatch request)
- Expected outcome (what success looks like for that step)
- Risk note (permissions, side-effects, irreversibility)

**Key constraint:** The plan is incremental. Only the **next action** is binding. The rest of the outline is advisory. This prevents long speculative plans from poisoning the run.

### 4. Act

**Reads:** `plan.md` (specifically the "next action" block)
**Purpose:** Execute the next step.

Act calls the Ductile API to trigger another plugin:
```
POST /api/trigger/<pipeline-or-plugin>
Authorization: Bearer <token-from-config>
Content-Type: application/json

{
  "event_type": "<appropriate-type>",
  "payload": { ... },
  "execution_mode": "synchronous"
}
```

The called plugin fires, does its work, returns a result, and terminates normally. AgenticLoop receives the synchronous HTTP response.

**Appends to `trace.jsonl`:**
```json
{
  "phase": "act",
  "step": 3,
  "timestamp": "ISO8601",
  "tool": "jina-reader",
  "args": {"url": "..."},
  "result_status": "ok",
  "result_summary": "...",
  "error": null,
  "retryable": null
}
```

**Design note:** Act is pure execution. No reasoning, no re-planning. Do the thing, record the evidence.

### 5. Reflect

**Reads:** `memory.md`, `plan.md`, `trace.jsonl` (latest entry)
**Purpose:** Decide whether we're done, continue, or reframe.

**Updates `memory.md`:**
- Progress update vs definition-of-done (checkboxes ticked)
- New facts learned (short bullets)
- Decision: `continue` | `done` | `reframe` | `escalate`
- If continue: what changed in priorities
- If done: final summary / output payload

**Updates `plan.md`:**
- Mark completed step
- Adjust remaining steps based on what was learned

**Decision routing:**
- `continue` → goto Plan
- `done` → exit loop, return final response via protocol v2
- `reframe` → goto Frame (goal was wrong or needs revision)
- `escalate` → exit loop with error/escalation status

**Stop conditions live here, not in the model's tool-calling behaviour.** Don't stop because the model "didn't call a tool." Stop because done-criteria are satisfied, or the budget is exhausted.

## Plugin Configuration

```yaml
plugins:
  agentic-loop:
    schedule: null                    # typically trigger-only, not scheduled
    timeout: 10m                      # generous timeout for multi-step work
    config:
      api_url: "http://localhost:${PORT}"
      api_token: "${DUCTILE_AGENT_TOKEN}"
      max_steps: 20                   # hard budget — Reflect enforces
      max_reframes: 2                 # prevent infinite reframing
      llm_provider: "anthropic"       # or ollama, openai, etc.
      llm_model: "claude-sonnet-4-5-20250929"
      allowed_plugins:                # skill whitelist (optional)
        - jina-reader
        - fabric
        - file_handler
        - youtube_transcript
```

### Token Scope

The AgenticLoop's API token should have a **bounded scope** — only the plugins listed in `allowed_plugins`. It must NOT have `admin:*` or be able to trigger itself (no recursive agent spawning without explicit configuration).

## Protocol v2 Response (on exit)

When the loop terminates (`done` or `escalate`), the plugin returns a standard protocol v2 response:

```json
{
  "status": "ok",
  "events": [
    {
      "type": "agent.completed",
      "payload": {
        "goal": "original goal statement",
        "outcome": "summary of what was achieved",
        "steps_taken": 7,
        "artifacts": ["report.md", "sources.json"]
      }
    }
  ],
  "state_updates": {
    "last_run": "ISO8601",
    "last_goal": "...",
    "total_steps": 7
  },
  "logs": [...]
}
```

This event can be routed to notification plugins (Discord, email, etc.) via normal pipeline routing.

## Budget & Safety

| Control | Enforced by | Mechanism |
|---------|-------------|-----------|
| Max wall-clock time | Core (timeout) | SIGTERM → grace → SIGKILL |
| Max loop iterations | Plugin (Reflect) | `max_steps` config, counter in memory |
| Max reframes | Plugin (Reflect) | `max_reframes` config, counter in memory |
| Allowed tools | Plugin (Act) | `allowed_plugins` whitelist in config |
| No self-invocation | Token scope | Token cannot trigger `agentic-loop` |
| Spend/cost tracking | Plugin (trace) | Logged per-step in `trace.jsonl` |

## What This Requires from Core

| Requirement | Status | Notes |
|-------------|--------|-------|
| Workspace directory | Exists | Standard workspace manager |
| Sync API endpoint | Exists | RFC-92 `execution_mode: synchronous` |
| API token auth | Exists | Token scopes already implemented |
| Plugin timeout config | Exists | Per-plugin timeout in config |
| `skills.md` generation | **New** | Core writes skill catalog to workspace before spawn |
| Longer timeout allowance | Config only | Set `timeout: 10m` in plugin config |

The only new core work is **skills.md generation** — iterating the plugin registry, formatting manifests into a markdown skill catalog, and writing it to the workspace before spawning the plugin.

## Implementation Language

Python is the natural choice (consistent with most existing plugins). The LLM interaction (Frame, Plan, Reflect are all LLM calls) can use any provider SDK. The Act phase is just HTTP calls to the Ductile API.

## Open Questions for Review

1. **skills.md generation** — should core do this (Option A), or should the plugin discover skills via API (Option B)? Option A is simpler but couples core to a markdown format. Option B is more self-contained but needs a new API endpoint.

2. **LLM provider abstraction** — should AgenticLoop hardcode one LLM provider, or support multiple via config? Suggest starting with one (Anthropic) and abstracting later if needed.

3. **Workspace persistence** — normal workspaces are cleaned up after retention period. Should agentic workspaces be retained longer for audit/replay? Or is `trace.jsonl` sufficient?

4. **Recursive agents** — should one AgenticLoop be able to spawn another? If so, how do we prevent infinite recursion? Suggest: forbid by default via token scope, allow with explicit `max_depth` config.

5. **Streaming progress** — the loop runs for minutes. Should it emit progress events mid-run (via the API), or is the final result sufficient? If progress is needed, the plugin could POST status updates to a webhook endpoint.

6. **Credential forwarding** — when AgenticLoop triggers another plugin, does it pass its own config (e.g., API keys for the target plugin)? No — the target plugin has its own config. AgenticLoop only sends the event payload.

## Success Criteria

- [ ] AgenticLoop plugin runs as a single process through multiple loop iterations
- [ ] Frame produces a bounded goal with definition of done
- [ ] Plan selects appropriate tools from skills.md
- [ ] Act successfully invokes other plugins via sync API and receives results
- [ ] Reflect correctly determines done/continue/reframe
- [ ] Budget controls (max_steps, timeout) prevent runaway execution
- [ ] trace.jsonl provides full audit trail of all actions and decisions
- [ ] Final result routes to downstream plugins via standard event routing

## References

- RFC-92: Pipeline Execution Modes (sync API infrastructure)
- RFC-004: LLM as Operator/Admin (skills concept, manifest extensions)
- SPEC.md §6: Protocol v2
- SPEC.md §8: Webhook / API endpoints
- Workspace Manager: `internal/workspace/interface.go`

## Narrative

- 2026-02-16: Initial RFC created. AgenticLoop as a long-running plugin using existing sync API infrastructure to invoke other plugins as tools. (by @claude)
