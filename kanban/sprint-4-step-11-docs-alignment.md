---
id: 67
status: todo
priority: High
blocked_by: [52, 56]
assignee: "@gemini"
tags: [documentation, plugin-v2, architecture]
---

# Documentation: Protocol v2 & Governance Hybrid Alignment

Update the core documentation to reflect the current implementation of Protocol v2 and the Governance Hybrid model.

## Acceptance Criteria
- `SPEC.md`: Update Section 6 (Protocol) to reflect v2 as the current standard.
- `SPEC.md`: Incorporate the Governance Hybrid (Control vs. Data Plane) into the Architecture overview.
- `USER_GUIDE.md`: Update "Plugin Development Guide" examples (Bash/Python) to use Protocol v2 (handling `workspace_dir` and `context`).
- `USER_GUIDE.md`: Add a section on "Operational Integrity" using `config check` and `config lock`.
- Ensure consistency between `PIPELINES.md` and the main `USER_GUIDE.md`.

## Narrative
- 2026-02-12: Created to align developer-facing docs with the Sprint 4 implementation. (by @gemini)
