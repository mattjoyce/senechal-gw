---
id: 18
status: done
priority: Normal
blocked_by: []
tags: [sprint-1, mvp, ops]
---

# PID Lock (Single Instance)

Ensure only one instance runs at a time using a PID file + `flock`.

## Acceptance Criteria
- Lock path based on state directory (SPEC: `<state_dir>/senechal-gw.lock`).
- Fails fast if lock cannot be acquired.
- Writes PID into the lock file while holding the lock.

## Narrative
- 2026-02-08: Implemented PID locking in `internal/lock/pidlock.go` with `AcquirePIDLock()` using Unix `flock` (LOCK_EX | LOCK_NB) for exclusive non-blocking lock acquisition. Lock file created at `<state_dir>/senechal-gw.lock` with current PID written to file. Returns error if lock already held by another process. Test verifies PID written correctly. Merged via PR #1. (by @codex)
