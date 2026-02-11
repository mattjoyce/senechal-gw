---
id: 58
status: todo
priority: High
blocked_by: [53, 54]
assignee: "@gemini"
tags: [maintenance, cli, refactor]
---

# Refactor CLI to NOUN VERB Hierarchy

Refactor `cmd/senechal-gw/main.go` to strictly enforce the NOUN VERB pattern defined in `docs/CLI_DESIGN_PRINCIPLES.md`.

---
Switch to to main, and rebase.
Create a branch gemini/cli-refactor
Work on Card #58
Commit only the code relating to the task.
Create a PR
---

## Acceptance Criteria
- Commands follow NOUN VERB:
    - `config lock` (replaces `hash-update`)
    - `job inspect <id>` (replaces `inspect <id>`)
    - `system start` (replaces `start`)
- Support root aliases for backward compatibility (e.g., `senechal-gw start` -> `system start`).
- Usage text reflects the new hierarchy.
- Unit tests updated to match new structure.

## Narrative
- 2026-02-12: Created to align with CLI Design Principles. (by @gemini)
