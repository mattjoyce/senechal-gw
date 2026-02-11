---
id: 49
status: doing
priority: High
blocked_by: [48]
assignee: "@gemini"
tags: [sprint-4, workspace, filesystem]
---

# Implement Filesystem Workspace Manager

Implement the concrete `fsWorkspaceManager` that handles the physical Data Plane (Artifacts).

**CRITICAL:** Read `docs/ROUTING_SPEC_GEMINI.md` before starting. Pay specific attention to the "Hardlink Clone" requirement for branch isolation.

---
Switch to to main, and rebase.
Create a branch codex/sprint-4-workspace
Work on Card #49
Commit only the code relating to the task.
Create a PR
---

## Acceptance Criteria
- `fsWorkspaceManager` implements the `workspace.Manager` interface.
- Support for **Hardlink Cloning** (`os.Link`) to ensure zero-copy branching.
- Basic directory management (creation under a configured base path).
- Unit tests for creation and cloning isolation.

## Narrative
- 2026-02-11: Initial card creation. (by @gemini)
- 2026-02-11: Implemented `fsWorkspaceManager` with job-scoped create/open/cleanup and hardlink-based clone semantics, plus tests covering workspace creation, hardlink fan-out behavior, and cleanup retention. (by @codex)
