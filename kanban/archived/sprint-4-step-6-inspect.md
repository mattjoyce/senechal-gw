---
id: 53
status: done
priority: Normal
blocked_by: [52]
assignee: "@codex"
tags: [sprint-4, cli, observability, lineage]
---

# Implement `ductile inspect` CLI Tool

Create a CLI utility to visualize the execution lineage of a multi-hop event chain, showing accumulated baggage and workspace artifacts.

---
Switch to to main, and rebase.
Create a branch codex/sprint-4-inspect
Work on Card #53
Commit only the code relating to the task.
Create a PR
---

## Acceptance Criteria
- `ductile inspect <job_id>` command.
- Fetches full lineage from `ContextStore.Lineage`.
- Displays a tree or list view of:
    - Step ID and Pipeline Name.
    - Accumulated Baggage (JSON metadata).
    - Workspace Artifacts (list files in the job's `workspace_dir`).
- Professional, monospace-friendly formatting for the terminal.

## Narrative
- 2026-02-11: Initial card creation. (by @gemini)
- 2026-02-11: Began implementation as prerequisite unblock for card #54; adding `ductile inspect <job_id>` to render context lineage, baggage, and workspace artifacts for a chain. (by @codex)
- 2026-02-11: Completed `inspect` command with lineage reporting from `ContextStore.Lineage`, structured baggage rendering, and workspace artifact listing per hop, including integration into CLI routing and unit tests for report formatting/content. (by @codex)
