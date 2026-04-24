# Ductile: Database Reference

Ductile uses **SQLite 3** for all persistent state, job queuing, and execution history. This document provides the schema definitions and a collection of useful queries for operators.

---

## Database Location
The database is typically named `ductile.db` and resides in your configured `state.path` (default: `~/.config/ductile/ductile.db`).

---

## Schema Overview

### Fact Rows vs Current Rows

Ductile keeps current/cache rows for fast operational reads and append-only
fact rows for durable explanation.

Current/cache rows:
- `job_queue`
- `plugin_state`
- `circuit_breakers`
- `schedule_entries`

Fact/history rows:
- `job_transitions`
- `job_attempts`
- `config_snapshots`
- `plugin_facts`
- `event_context`
- `job_log`
- `circuit_breaker_transitions`

`storage_sequences` is an internal allocator for Ductile-owned fact ordering.
New `plugin_facts` rows receive a monotonic `seq`. Legacy `plugin_facts` rows
may have `seq IS NULL`; those rows keep their old timestamp-only ordering and
should not be treated as perfectly ordered facts.

### 1. `job_queue`
The active work queue. Contains pending, running, and recently completed jobs.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Unique job identifier. |
| `status` | TEXT | `queued`, `running`, `succeeded`, `failed`, `timed_out`, `dead`. |
| `plugin` | TEXT | Name of the plugin or alias. |
| `command` | TEXT | The plugin command (e.g., `poll`, `handle`). |
| `payload` | JSON | Input data for the plugin. |
| `dedupe_key` | TEXT | Used to prevent duplicate enqueues. |
| `event_context_id`| UUID | Reference to the baggage/context for this job. |

### 2. `job_log`
The historical record of completed jobs. Used for auditing and the TUI "Overwatch."

| Column | Type | Description |
|--------|------|-------------|
| `result` | JSON | The full protocol response from the plugin. |
| `stderr` | TEXT | Captured stderr (capped at 64 KB). |
| `last_error` | TEXT | Human-readable error message if the job failed. |

### 3. `event_context`
The "Control Plane" ledger. Stores metadata (Baggage) that propagates through pipelines.

### 4. `plugin_state`
Persistent plugin compatibility/cache row. Some plugins still write directly to
it, while fact-migrated plugins derive it from declared `plugin_facts`.

### 5. `plugin_facts`
Append-only plugin observations. `plugin_state` remains the
compatibility/current-state row for existing runtime reads.

For existing databases, apply required schema migrations before deploy. Startup
validation reports the migration script to run when the database is behind the
runtime schema.

### 6. `schedule_entries`
The persistent state of the scheduler. Tracks when each schedule last fired and when it is due next.

### 7. `circuit_breakers`
Current-state compatibility/cache row for scheduled poll circuit breakers.

### 8. `circuit_breaker_transitions`
Append-only transition facts for circuit breakers. Use this table to explain why a breaker opened, moved half-open, closed, or was manually reset.

---

## Useful Operator Queries

### System Health
```sql
-- Count jobs by status
SELECT status, COUNT(*) 
FROM job_queue 
GROUP BY status;

-- Identify plugins with active circuit breakers
SELECT plugin, command, state, failure_count, opened_at 
FROM circuit_breakers 
WHERE state != 'closed';

-- Show recent breaker transitions
SELECT created_at, plugin, command, from_state, to_state, reason, failure_count, job_id
FROM circuit_breaker_transitions
WHERE plugin = 'my-plugin' AND command = 'poll'
ORDER BY created_at DESC
LIMIT 20;

-- Check for stuck "running" jobs (orphans)
SELECT id, plugin, command, started_at 
FROM job_queue 
WHERE status = 'running' 
  AND started_at < datetime('now', '-1 hour');
```

### Performance & Troubleshooting
```sql
-- Find the slowest successful jobs in the last 24 hours
SELECT plugin, command, 
       (strftime('%s', completed_at) - strftime('%s', started_at)) as duration_sec
FROM job_log
WHERE status = 'succeeded'
  AND completed_at > datetime('now', '-1 day')
ORDER BY duration_sec DESC
LIMIT 10;

-- Get the latest error for a specific plugin
SELECT completed_at, last_error, stderr
FROM job_log
WHERE plugin = 'my-plugin' AND status = 'failed'
ORDER BY completed_at DESC
LIMIT 1;

-- Inspect a plugin's persistent state
SELECT state FROM plugin_state WHERE plugin_name = 'my-plugin';

-- Inspect recent append-only plugin facts
SELECT seq, created_at, fact_type, job_id, command, fact_json
FROM plugin_facts
WHERE plugin_name = 'file_watch'
ORDER BY
  CASE WHEN seq IS NULL THEN 1 ELSE 0 END ASC,
  seq DESC,
  created_at DESC
LIMIT 20;
```

### Scheduler Inspection
```sql
-- See upcoming scheduled runs
SELECT plugin, schedule_id, next_run_at, last_success_at
FROM schedule_entries
WHERE status = 'active'
ORDER BY next_run_at ASC;
```

---

## Maintenance

### Manual Cleanup
Ductile automatically prunes `job_log` after 30 days, but you can manually vacuum or prune if needed:
```bash
# Prune logs older than 7 days
sqlite3 ductile.db "DELETE FROM job_log WHERE completed_at < datetime('now', '-7 days');"

# Reclaim disk space
sqlite3 ductile.db "VACUUM;"
```

### Performance Tuning
Ductile enables **WAL mode** and **Synchronous=NORMAL** by default for optimal performance on SSDs. You can verify this via:
```sql
PRAGMA journal_mode;
PRAGMA synchronous;
```
