# SQL Tightening Log

Audit-trail entries for schema changes shipped under the SQL Tightening plan.
Each entry names the wave, the change, the rationale, and the revisit trigger
if the deferral / drop turns out to need reversing.

The plan itself lives outside the repo (operator's PRD); this file is the
in-tree record of what landed and what was deliberately not done.

---

## Wave 2.E — drop nine unused indexes (2026-05-06)

Dropped nine indexes that no production query targets. Each was verified by
grep across `internal/**/*.go` and `cmd/ductile/**/*.go` for any
`WHERE`, `ORDER BY`, `GROUP BY`, or `JOIN` clause using the indexed column
or composite. None matched.

| Index | Table | Why dropped | Re-add trigger |
|---|---|---|---|
| `job_queue_enqueued_config_snapshot_idx` | job_queue | Only INSERT/UPDATE writes to this column; no read-side filter | Future audit query "list jobs admitted under config snapshot X" |
| `job_queue_started_config_snapshot_idx` | job_queue | Same | Future audit query "list jobs that started under config snapshot X" |
| `config_snapshots_loaded_at_idx` | config_snapshots | Only `WHERE id = ?` (PK) lookups; tiny table | Future "list recent snapshots" admin query |
| `plugin_facts_job_id_idx` | plugin_facts | Only `WHERE plugin_name [AND fact_type]` queries; was in `requiredIndexes` defensively | Future "show all facts for job X" debug path. Removed from `requiredIndexes` in `internal/storage/sqlite.go` as part of this wave. |
| `event_context_parent_id_idx` | event_context | Recursive lineage CTE walks via `JOIN ... ON lineage.parent_id = ec.id` — lookup uses `ec.id` (PK), not `ec.parent_id` | Future query that filters `WHERE parent_id = ?` directly |
| `job_log_enqueued_config_snapshot_idx` | job_log | No backing query | Same as `job_queue` equivalent |
| `job_log_started_config_snapshot_idx` | job_log | No backing query | Same |
| `circuit_breaker_transitions_job_idx` (partial, `WHERE job_id IS NOT NULL`) | circuit_breaker_transitions | No `WHERE job_id = ?` query in production code | Future "show all breaker transitions caused by job X" path |
| `schedule_entries_status_idx` | schedule_entries | Only `WHERE plugin = ? AND schedule_id = ?` (PK) lookups; status is a bounded enum on a tiny table | Future bulk filter `WHERE status = 'active'` on a much larger schedule set |

### What stayed

Sixteen indexes remain. Each has a named backing query — see PRD iteration 8
audit matrix for the full mapping.

`job_queue_event_source_idx` UNIQUE was kept and a comment added in
`schema.sql` flagging that it carries an integrity guarantee, not just a
speed optimisation: it prevents duplicate child enqueue per
`(parent_job_id, source_event_id)`.

### Migration

`scripts/migrate-drop-unused-indexes.py <db>` is idempotent. `DROP INDEX
IF EXISTS` is a SQLite metadata operation — no table rebuild, hot-safe.

### Deploy ordering

For Mac: binary swap (with the updated `requiredIndexes` set) then run the
migration. Reverse order — migration first, old binary still expecting
`plugin_facts_job_id_idx` — would refuse to start at next restart.

For Thinkpad / Unraid: same ordering. Each instance migrates independently.

### Pre-existing audit finding (not addressed in 2.E)

`internal/inspect/report.go:312` does `WHERE event_context_id = ?` on
`job_queue` with no index covering `event_context_id`. This is already a
full-table scan today — slow on large datasets. Not introduced by 2.E; flagged
here for a future iteration to either add `job_queue_event_context_id_idx` (if
inspect-by-context is a hot enough path to justify the write cost) or to accept
that `inspect` is an admin-only path with relaxed performance expectations.

---
