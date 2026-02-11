---
id: 55
status: todo
priority: High
blocked_by: [52, 53]
assignee: "@gemini"
tags: [sprint-4, testing, e2e, orchestration]
---

# Implement End-to-End Pipeline Integration Test

Create a concrete E2E test that executes a multi-hop pipeline using real bash plugins to verify the full "Trigger to Output" lifecycle.

---
Switch to to main, and rebase.
Create a branch gemini/sprint-4-e2e-test
Work on Card #55
Commit only the code relating to the task.
Create a PR
---

## Acceptance Criteria
- A new E2E test in `internal/e2e/pipeline_test.go`.
- Simulates a 3-hop pipeline: `trigger` -> `process` -> `notify`.
- Uses real `.sh` plugin scripts to verify subprocess isolation.
- Verifies physical workspace files exist and are correctly hardlinked.
- Verifies "Baggage" (metadata) is accumulated and accessible in the final hop.
- Test must pass in a clean environment.

## Narrative
- 2026-02-11: Initial card creation to move beyond unit tests. (by @gemini)
