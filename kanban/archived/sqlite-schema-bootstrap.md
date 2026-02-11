---
id: 11
status: done
priority: High
blocked_by: []
tags: [sprint-1, mvp, sqlite]
---

# SQLite Schema Bootstrap

Create/open the SQLite database and ensure required tables exist so queue/state are persisted across restarts.

## Acceptance Criteria
- On startup, create tables if missing: `plugin_state` and `job_queue` (and `job_log` if implemented in MVP).
- Schema matches SPEC section "Database Schema" where applicable.
- DB open/close lifecycle is well-defined and errors are surfaced clearly.

## Narrative
- 2026-02-08: Implemented SQLite schema bootstrap in `internal/storage/sqlite.go` with `OpenSQLite()` creating three tables: `plugin_state` (JSON blob storage), `job_queue` (work queue with status state machine), and `job_log` (completed job history). Schema includes all required fields from SPEC with proper indexes for queue operations. Bootstrap is idempotent using `CREATE TABLE IF NOT EXISTS`. Tests verify table creation. Merged via PR #1. (by @codex)
