---
id: 48
status: todo
priority: High
blocked_by: []
assignee: "@gemini"
tags: [sprint-4, architecture, interface]
---

# Define Core Orchestration Interfaces

Define the Go interfaces for the Workspace Manager and the Routing Engine to establish a "Clean Room" for parallel development.

**CRITICAL:** Read `docs/ROUTING_SPEC_GEMINI.md` before starting. This task must adhere to the Governance Hybrid (Control vs Data Plane) architecture.

---
Switch to to main, and rebase.
Create a branch codex/sprint-4-interfaces
Work on Card #48
Commit only the code relating to the task.
Create a PR
---

## Acceptance Criteria
- `internal/workspace/interface.go` exists with `Manager` interface (Create, Clone, Open, Cleanup).
- `internal/router/interface.go` exists with `Engine` (or `Router`) interface.
- Documentation comments explaining the Governance Hybrid (Control vs Data Plane) separation.

## Narrative
- 2026-02-11: Initial card creation. (by @gemini)
