---
id: 126
status: todo
priority: High
blocked_by: []
tags: [bug, scheduler, dedupe, queue]
---

# Bug: Scheduler Dedupe TTL Blocks Frequent Schedules

## Symptom

Scheduled jobs stop firing after the first run and don't resume for 24 hours.
Observed in production with `withings` plugin schedules (`token_refresh` every 20m,
`data_fetch` every 6h). The scheduler logs show repeated `dropped duplicate enqueue`
messages referencing the same completed job ID.

Schedule entries (`schedule_entries` table) show `next_run_at` stuck in the past,
and the TUI hides those schedules because `next_run_at` is not in the future.

## Root Cause

### Location
`internal/queue/queue.go` — `findRecentSucceededByDedupeKey` (line 156)

### What it does
When a job is enqueued with a `DedupeKey`, the queue checks for a recent **succeeded**
job with the same key within the dedupe TTL window:

```go
// queue.go:46 — TTL is hardcoded
dedupeTTL: 24 * time.Hour,

// queue.go:156-168 — blocks on any succeeded job within TTL
func (q *Queue) findRecentSucceededByDedupeKey(...) {
    cutoff := time.Now().UTC().Add(-ttl).Format(time.RFC3339Nano)
    err := q.db.QueryRowContext(ctx, `
SELECT id FROM job_queue
WHERE dedupe_key = ?
  AND status = ?
  AND completed_at IS NOT NULL
  AND completed_at >= ?
ORDER BY completed_at DESC LIMIT 1;
`, dedupeKey, StatusSucceeded, cutoff).Scan(&id)
```

### What the scheduler does
`internal/scheduler/scheduler.go:339` — builds the dedupe key and passes it to enqueue:

```go
dedupeKey := fmt.Sprintf("%s:%s:%s", pluginName, command, scheduleID)
// ...
DedupeKey: &dedupeKey,
```

The scheduler does **not** pass a custom TTL — it uses the queue's global 24h default.

### Result
After a scheduled job succeeds, the same schedule is blocked from firing for 24 hours,
regardless of the configured `every` interval. A 20-minute schedule effectively becomes
a once-per-day schedule.

Additionally, when dedupe blocks an enqueue, `next_run_at` in `schedule_entries` is NOT
advanced — it stays at the originally-computed future time and becomes permanently stale
after one dedupe hit. This causes the TUI countdown to disappear for affected schedules.

## Proposed Fix

### Option A — Per-enqueue dedupe TTL (preferred)

Allow callers to override the dedupe TTL per enqueue request. The scheduler passes
`TTL = baseInterval` (the schedule's `every` value) so dedupe only prevents double-firing
within one interval, not for 24 hours.

Changes:
- Add optional `DedupeTTL *time.Duration` to `queue.EnqueueRequest` (or similar)
- In `Queue.Enqueue`, use `req.DedupeTTL` when set, fall back to `q.dedupeTTL`
- In `scheduler.go`, pass `DedupeTTL = baseInterval` when building the enqueue request

### Option B — Only dedupe on pending/running jobs

Change `findRecentSucceededByDedupeKey` to only block on jobs that are currently
`queued` or `running`, not `succeeded`. Completed jobs would never block re-enqueue.

This is semantically cleaner (dedupe = prevent concurrent duplicates, not rate-limit),
but requires renaming/replacing the function and updating the query:

```sql
SELECT id FROM job_queue
WHERE dedupe_key = ?
  AND status IN ('queued', 'running')
  AND created_at >= ?
LIMIT 1;
```

**Recommendation**: Option B is the right semantic fix. The purpose of dedupe for
scheduler jobs is to prevent two instances of the same schedule from running
concurrently — not to enforce a cooldown. Option A is a workaround that preserves
broken semantics.

## Secondary Bug: next_run_at not advanced on dedupe hit

When the scheduler hits a dedupe and skips enqueue, it should still advance
`next_run_at` in `schedule_entries` to `now + jittered_interval`. Currently it does
not, causing the entry to appear stuck in the past permanently.

Fix: in the scheduler's dedupe-hit branch (around `scheduler.go:363`), call the same
`next_run_at` update that runs after a successful enqueue.

## Reproduction

1. Configure a withings-style plugin with `every: 20m` schedule
2. Let it fire once (job succeeds, dedupe_key is set on the job_queue row)
3. Wait 21 minutes — scheduler tries to enqueue but hits dedupe on the succeeded job
4. Observe: no new job created, `next_run_at` stuck, TUI shows no countdown for that schedule
5. Workaround: `UPDATE job_queue SET dedupe_key = NULL WHERE id = '<blocking-job-id>';`
   and `UPDATE schedule_entries SET next_run_at = strftime('%Y-%m-%dT%H:%M:%SZ', datetime('now', '-1 second')) WHERE plugin = 'withings';`

## Acceptance Criteria

- [ ] A 20-minute schedule fires every ~20 minutes (not once per 24h)
- [ ] Dedupe still prevents two concurrent runs of the same scheduled job
- [ ] `next_run_at` in `schedule_entries` is always advanced after each scheduler evaluation, even on dedupe hit
- [ ] TUI schedule countdown remains visible and accurate
- [ ] Existing queue dedupe behaviour for API-submitted jobs is unchanged
