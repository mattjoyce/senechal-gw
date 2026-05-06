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

## Wave 2.F — EXPLAIN QUERY PLAN regression test (2026-05-06)

`internal/storage/explain_test.go` pins the SQLite query plan for each hot
production query. The 2.E index audit relied on grep against query sites; 2.F
is the structural safety net so a future schema change that breaks one of the
kept indexes fails `go test ./...` instead of becoming a silent production
slowdown.

### Pinned cases

| Case | Hot table | Backing index |
|---|---|---|
| `claim_next` | `job_queue` | `job_queue_status_created_at_idx` (candidate scan) + `job_queue_dedupe_status_completed_idx` (running subquery) |
| `list_jobs_by_status` | `job_queue` | `job_queue_status_created_at_idx` |
| `list_job_logs` | `job_log` | `job_log_completed_at_idx` (filter) + `job_log_job_id_attempt_idx` is the LEFT JOIN counterpart (LEFT-JOIN side uses `job_queue` PK autoindex on `id`) |
| `breaker_lookup` | `circuit_breakers` | `sqlite_autoindex_circuit_breakers_1` (composite PK) |
| `schedule_lookup` | `schedule_entries` | `sqlite_autoindex_schedule_entries_1` (composite PK) |
| `plugin_facts_by_plugin_seq` | `plugin_facts` | `plugin_facts_plugin_seq_idx` (WHERE-side; ORDER BY uses TEMP B-TREE due to the `CASE WHEN seq IS NULL` sort key) |

### Assertion shape

For each case the test:

1. Captures `EXPLAIN QUERY PLAN <query>` against a freshly bootstrapped
   schema and normalises rows to `id|parent|detail` lines (the fourth column
   is dropped to keep snapshots stable across SQLite minor versions).
2. Hard-asserts the plan does NOT contain `\bSCAN <hot_table>\b` — the named
   base table must be searched, not full-scanned. Subqueries and CTE-derived
   rows are exempt.
3. Hard-asserts the plan contains the expected index substring.
4. Compares the normalised plan text against
   `internal/storage/testdata/explain/<case>.golden`.

### Refreshing goldens

`UPDATE_GOLDEN=1 go test ./internal/storage -run TestExplainQueryPlanRegression`
overwrites the snapshot files. Use only after an intentional query or schema
change. The hard `no SCAN <table>` and `expectIndex` assertions still run, so
a regenerated golden cannot mask a missing index.

### What this catches

- Dropped index in `schema.sql` that a hot query relied on
- Renamed column that breaks an existing covering index
- ORDER BY change that forces a temp B-tree where one was avoided
- A new query added to a hot path with no covering index

### What this does not catch

- Performance regressions that keep the same index but degrade for other
  reasons (row count, page cache pressure, planner heuristic shifts in a
  major SQLite bump)
- Queries not in the pinned set — extending coverage means adding a case;
  the test is intentionally explicit per Hickey doctrine, no auto-discovery

---
