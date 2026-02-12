---
id: 67
status: done
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
- `USER_GUIDE.md`: Document that `POST /trigger/{plugin}/handle` payloads are automatically wrapped in an `api.trigger` event (Card #68).
- `USER_GUIDE.md`: Document the new `config show`, `config get`, and `config set` affordances for surgical system administration (Cards #69, #70).
- `USER_GUIDE.md`: Document flexible scheduler intervals including Go durations plus `d`/`w` suffixes (`7m`, `13h`, `3d`, `2w`) and aliases (`hourly`, `daily`, `weekly`, `monthly`) from Card #82.
- Ensure consistency between `PIPELINES.md` and the main `USER_GUIDE.md`.

## Narrative
- 2026-02-12: Created to align developer-facing docs with the Sprint 4 implementation. (by @gemini)
- 2026-02-13: Added docs task to capture flexible scheduler interval support introduced in card #82 so user-facing docs reflect new schedule options. (by @assistant)
- 2026-02-13: Moved to 'doing'. Starting updates for SPEC.md and USER_GUIDE.md. (by @gemini)
- 2026-02-13: Completed all updates. SPEC.md reflects Protocol v2 and Governance Hybrid. USER_GUIDE.md updated with examples and new CLI features. API_REFERENCE.md created. (by @gemini)
