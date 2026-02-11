---
id: 50
status: todo
priority: High
blocked_by: [48, 49]
assignee: "@codex"
tags: [sprint-4, control-plane, sqlite, baggage]
---

# Implement Event Context (The Ledger)

Implement the `event_context` table and storage logic in SQLite to support the "Baggage" and "Execution Ledger" features of the Governance Hybrid model.

**CRITICAL:** Read `docs/ROUTING_SPEC_GEMINI.md` before starting. Pay specific attention to "Context Accumulation" and the "Origin Anchor" requirements.

---
Switch to to main, and rebase.
Create a branch codex/sprint-4-ledger
Work on Card #50
Commit only the code relating to the task.
Create a PR
---

## Acceptance Criteria
- SQLite migration for `event_context` table per SPEC §3.1 (id, parent_id, pipeline_name, step_id, accumulated_json, created_at).
- Update `job_queue` and `job_log` tables to include `event_context_id`.
- Implement `ContextStore` in `internal/state` to handle context creation and lineage retrieval.
- Logic to merge new metadata while preserving "Origin Anchor" (immutable keys starting with `origin_`).
- Enforcement of the 1MB limit on `accumulated_json` (Baggage Overflow protection).

## Narrative
- 2026-02-11: Initial card creation. (by @gemini)
