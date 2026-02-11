---
id: 52
status: done
priority: High
blocked_by: [49, 50, 51]
assignee: "@codex"
tags: [sprint-4, router, dispatch, integration]
---

# Implement Orchestration Router & Dispatcher Wiring

Wire the Router Engine into the Dispatcher loop to enable automatic enqueuing of downstream jobs based on pipeline definitions.

**CRITICAL:** Read `docs/ROUTING_SPEC_GEMINI.md` before starting. This is the final integration of the Governance Hybrid model.

---
Switch to to main, and rebase.
Create a branch codex/sprint-4-integration
Work on Card #52
Commit only the code relating to the task.
Create a PR
---

## Acceptance Criteria
- Concrete `Router` implementation in `internal/router` that uses the `dsl.Compiler`.
- Update `internal/dispatch` to trigger the `Router` after every successful job.
- Implement the **Multi-Event Branching** pattern: match emitted `event.type` to pipeline steps.
- Automatic **Workspace Cloning** (hard-links) via the `WorkspaceManager` for every downstream job.
- Automatic **Context Accumulation** via the `ContextStore` for every downstream job.
- Proper propagation of `parent_job_id` and `event_context_id` for full traceability.
- Integration test demonstrating a 2-hop chain (e.g., A -> B) with baggage preservation.

## Narrative
- 2026-02-11: Initial card creation. (by @gemini)
- 2026-02-11: Started router/dispatcher integration: wiring compiled pipeline routing into successful job handling with downstream workspace clones, event-context accumulation, and traceability propagation. (by @codex)
- 2026-02-11: Completed orchestration wiring with a concrete DSL-backed router, dispatcher-side downstream enqueue flow, per-hop context accumulation (`event_context`), workspace clone fan-out, and traceability propagation (`parent_job_id`/`event_context_id`/`source_event_id`), plus integration coverage proving a 2-hop A→B chain with baggage and artifact preservation. (by @codex)
