---
id: 18
status: todo
priority: Normal
blocked_by: [9, 11]
tags: [sprint-1, mvp, ops]
---

# PID Lock (Single Instance)

Ensure only one instance runs at a time using a PID file + `flock`.

## Acceptance Criteria
- Lock path based on state directory (SPEC: `<state_dir>/senechal-gw.lock`).
- Fails fast if lock cannot be acquired.
- Writes PID into the lock file while holding the lock.

## Narrative

