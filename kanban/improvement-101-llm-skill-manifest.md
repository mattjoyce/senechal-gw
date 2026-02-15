---
id: 101
status: done
priority: Normal
blocked_by: []
assignee: "@gemini"
tags: [llm, documentation, rfc-004]
---

# Develop SKILL.md for LLM Operators

Create a specialized manifest that exposes Ductile's CLI and plugin capabilities in a format optimized for LLM system prompts.

## Acceptance Criteria
- [x] Define the structure of `SKILL.md` (aligned with RFC-004).
- [x] Document core gateway utilities (config, system, job).
- [x] Add a command to export current plugin registry as Skill definitions.
- [x] Verify the manifest is "LLM-parsable" and concise.

## Narrative
- 2026-02-15: Created to fulfill RFC-004 Section 9. (by @gemini)
- 2026-02-15: Implemented `SKILL.md` template and `ductile system skills` command to auto-generate plugin capability documentation. (by @gemini)
