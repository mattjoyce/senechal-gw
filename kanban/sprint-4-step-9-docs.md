---
id: 56
status: done
priority: High
blocked_by: [55]
assignee: "@gemini"
tags: [sprint-4, documentation, pipelines]
---

# Create Comprehensive Pipeline & Routing Documentation

Write a standalone guide explaining how to design, build, and debug event-driven pipelines in Ductile.

---
Switch to to main, and rebase.
Create a branch gemini/sprint-4-docs
Work on Card #56
Commit only the code relating to the task.
Create a PR
---

## Acceptance Criteria
- `docs/PIPELINES.md` exists and is comprehensive.
- Explains the **Governance Hybrid** model (Baggage vs. Workspaces).
- Documents the **Pipeline DSL** syntax with clear examples.
- Explains the **Multi-Event Branching** pattern (logic in plugins, not DSL).
- Includes a "Troubleshooting" section using the `inspect` tool.
- Provides a walk-through of the "Video Wisdom" example.

## Narrative
- 2026-02-11: Initial card creation to formalize orchestration knowledge. (by @gemini)
- 2026-02-13: Finalized PIPELINES.md with comprehensive documentation on the Governance Hybrid model, Pipeline DSL, Multi-Event Branching, and Troubleshooting. Ensured alignment with Protocol v2 and updated USER_GUIDE.md for consistency. (by @gemini)
