---
id: 25
status: done
priority: High
blocked_by: []
tags: [sprint-1, mvp, ops]
---

# Crash Recovery Implementation

Implement the agreed crash recovery behavior on startup for orphaned `running` jobs.

## Acceptance Criteria
- On startup, scan for `status = running` jobs and apply the chosen policy.
- Each recovered job is logged at WARN with `job_id`, `plugin`, `command`, and outcome.
- Behavior is covered in the E2E runbook (kill -9, restart, observe recovery).

## Narrative
- 2026-02-08: Implemented `recoverOrphanedJobs()` in `internal/scheduler/scheduler.go` following Option B (SPEC semantics): orphaned jobs are re-queued if under `max_attempts`, or marked `dead` if attempts exhausted. Method is called during `Start()` before tick loop begins. Uses `QueueService.FindJobsByStatus()` to locate orphaned jobs and `UpdateJobForRecovery()` to transition them. Comprehensive tests cover re-queue path, dead path, and error handling. Merged via PR #6. (by @gemini)
