---
id: 11
status: todo
priority: High
blocked_by: [9]
tags: [sprint-1, mvp, sqlite]
---

# SQLite Schema Bootstrap

Create/open the SQLite database and ensure required tables exist so queue/state are persisted across restarts.

## Acceptance Criteria
- On startup, create tables if missing: `plugin_state` and `job_queue` (and `job_log` if implemented in MVP).
- Schema matches SPEC section "Database Schema" where applicable.
- DB open/close lifecycle is well-defined and errors are surfaced clearly.

## Narrative

