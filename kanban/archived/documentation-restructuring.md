---
id: 98
status: done
priority: Normal
blocked_by: []
assignee: "@gemini"
tags: [documentation, rfc-004, refactor]
---

# Documentation Restructuring (RFC-004 Alignment)

Reorganize documentation into role-based sections to improve coherence for both human users and LLM operators, following the vision defined in RFC-004.

## Acceptance Criteria
- [x] Root `README.md` updated as a high-level portal and documentation index.
- [x] `docs/GETTING_STARTED.md` created (from `USER_GUIDE.md` sections 1 & 2).
- [x] `docs/OPERATOR_GUIDE.md` created (from `USER_GUIDE.md` sections 4, 6, 7 + CLI principles).
- [x] `docs/PLUGIN_DEVELOPMENT.md` created (from `USER_GUIDE.md` section 5 + v2 protocol).
- [x] `docs/CONFIG_SPEC.md` renamed/moved to `docs/CONFIG_REFERENCE.md`.
- [x] `SPEC.md` renamed/moved to `docs/ARCHITECTURE.md`.
- [x] `docs/TESTING.md` created (merging `CLI_TEST_PLAN.md` and `E2E_ECHO_RUNBOOK.md`).
- [x] `docs/USER_GUIDE.md` and other redundant files removed.
- [x] All internal cross-document links updated and verified.

## Narrative
- 2026-02-14: Created task to move away from a monolithic user guide toward role-based manuals (User, Operator, Developer, QA). (by @gemini)
- 2026-02-14: Completed restructuring. Migrated content to role-based guides, updated root README index, and verified all internal links. (by @gemini)
