---
id: 52
status: todo
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
