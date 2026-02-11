---
id: 51
status: todo
priority: High
blocked_by: [48, 50]
assignee: "@codex"
tags: [sprint-4, router, dsl, compiler]
---

# Implement Pipeline DSL & Compiler

Build the engine that parses YAML pipeline definitions into validated Directed Acyclic Graphs (DAGs) for execution.

**CRITICAL:** Read `docs/ROUTING_SPEC_GEMINI.md` before starting. Pay specific attention to "Pipeline DSL" and "Cycle Detection."

---
Switch to to main, and rebase.
Create a branch codex/sprint-4-dsl
Work on Card #51
Commit only the code relating to the task.
Create a PR
---

## Acceptance Criteria
- `internal/router/dsl` package for parsing and compilation.
- YAML schema support for `on` (trigger), `steps` (sequential), `call` (nested), and `split` (parallel).
- DAG validation logic to detect and reject circular references at load time.
- BLAKE3 fingerprinting of compiled pipelines to support version pinning in the execution ledger.
- Multi-file loader that discovers and compiles all `.yaml` files in the `pipelines/` config subdirectory.
- Unit tests for valid graphs, circular dependency rejection, and nesting.

## Narrative
- 2026-02-11: Initial card creation. (by @gemini)
