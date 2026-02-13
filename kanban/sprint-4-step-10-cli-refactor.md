---
id: 58
status: done
priority: High
blocked_by: [53, 54]
assignee: "@gemini"
tags: [maintenance, cli, refactor]
---

# Refactor CLI to NOUN ACTION Hierarchy

Refactor `cmd/ductile/main.go` to strictly enforce the NOUN ACTION pattern defined in `docs/CLI_DESIGN_PRINCIPLES.md`.

---
Switch to to main, and rebase.
Create a branch gemini/cli-refactor
Work on Card #58
Commit only the code relating to the task.
Create a PR
---

## Acceptance Criteria
- Commands follow NOUN ACTION:
    - `config lock` (replaces `hash-update`)
    - `job inspect <id>` (replaces `inspect <id>`)
    - `system start` (replaces `start`)
- Support root aliases for backward compatibility (e.g., `ductile start` -> `system start`).
- Usage text reflects the new hierarchy.
- Unit tests updated to match new structure.

## Narrative
- 2026-02-12: Created to align with CLI Design Principles. (by @gemini)
