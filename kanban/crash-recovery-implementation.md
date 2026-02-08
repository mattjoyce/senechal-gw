---
id: 25
status: todo
priority: High
blocked_by: [24, 11, 14]
tags: [sprint-1, mvp, ops]
---

# Crash Recovery Implementation

Implement the agreed crash recovery behavior on startup for orphaned `running` jobs.

## Acceptance Criteria
- On startup, scan for `status = running` jobs and apply the chosen policy.
- Each recovered job is logged at WARN with `job_id`, `plugin`, `command`, and outcome.
- Behavior is covered in the E2E runbook (kill -9, restart, observe recovery).

## Narrative

