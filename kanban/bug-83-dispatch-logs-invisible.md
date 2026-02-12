---
id: 83
status: done
priority: High
tags: [bug, logging, observability, dispatch, testing]
---

# BUG: Dispatch execution logs not visible in stdout

## Description

Job execution logs from the dispatch component are not emitted to stdout, making real-time monitoring impossible. Only startup logs appear, while job execution happens silently even though jobs complete successfully.

## Impact

- **Severity**: High - Blocks real-time observability
- **Scope**: All job execution (poll, handle commands)
- **User Experience**: Cannot observe gateway activity without querying database

## Evidence

**Test**: TP-001 Schedule Trigger (2026-02-13)

Gateway started with `log_level: info`, echo plugin configured for 5m schedule.

**Stdout output** (complete):
```
19 lines total - only startup logs:
- senechal-gw starting
- PID lock acquired
- Database opened
- 3 plugins loaded
- API server starting
- Scheduler started
- Dispatch loop started
- Jobs enqueued (3 poll jobs)
... then nothing
```

**Database evidence** (jobs DID execute):
```sql
sqlite3 data/state.db "SELECT id, plugin, status, created_at FROM job_queue WHERE plugin='echo';"

3240f775-...|echo|succeeded|2026-02-12T18:55:32.118Z
db5e4938-...|echo|succeeded|2026-02-12T18:56:32.122Z
```

**Job log confirmed**:
```json
{
  "status": "ok",
  "logs": [{
    "level": "info",
    "message": "Test echo - 5m schedule at 2026-02-12T18:56:32Z"
  }]
}
```

## Expected Behavior

Following standard practice (and docs), execution logs should appear in stdout:

```json
{"time":"...","level":"INFO","component":"dispatch","msg":"job started","plugin":"echo","job_id":"3240f775-..."}
{"time":"...","level":"INFO","component":"dispatch","msg":"plugin executing","plugin":"echo","job_id":"3240f775-..."}
{"time":"...","level":"INFO","component":"dispatch","msg":"job completed","plugin":"echo","job_id":"3240f775-...","status":"succeeded","duration":"1.028s"}
```

From USER_GUIDE.md:
> You should see log entries indicating the scheduler is running and, after the configured interval, the echo plugin executing.

## Root Cause (Hypothesis)

**Likely**: Dispatch component logs at DEBUG level, filtered out by INFO threshold.

**Evidence**:
- Startup logs show: `"component":"main"` and `"component":"scheduler"` (visible at INFO)
- Dispatch log shows: `"component":"dispatch"` then silence
- Common pattern: execution details logged at DEBUG for performance

**Code location** (suspected): `internal/dispatch/worker.go` or similar

## LLM Operator / Testing Impact

**Critical for**:
1. **UAT Testing**: Cannot observe job execution in real-time
2. **Debugging**: Must query database to confirm jobs ran
3. **Monitoring**: No live visibility into gateway activity
4. **Development**: Slows debugging cycle (no immediate feedback)

**Current workaround**: Query database repeatedly
```bash
watch -n 1 'sqlite3 data/state.db "SELECT plugin, status, created_at FROM job_queue ORDER BY created_at DESC LIMIT 5;"'
```

## Reproduction

```bash
# Start gateway with INFO logging
./senechal-gw system start --config config.yaml 2>&1 | tee gateway.log

# In another terminal, check line count
watch -n 5 'wc -l gateway.log'
# Result: Count stops growing after startup (~19 lines)

# Confirm jobs ARE executing
sqlite3 data/state.db "SELECT COUNT(*) FROM job_queue WHERE status='succeeded';"
# Result: Count increases over time
```

## Testing Recommendations

**Quick verification**:
1. Set `log_level: debug` in config.yaml
2. Start gateway
3. Check if execution logs now appear

**If hypothesis correct**:
- Dispatch component should log job lifecycle events at INFO level
- DEBUG can be used for detailed execution traces
- User guide promises echo execution will be logged

**After fix, verify**:
- [ ] Job started log visible at INFO level
- [ ] Job completed log visible at INFO level
- [ ] Plugin name, job_id, status included in logs
- [ ] Duration/timing info included
- [ ] Works for both poll and handle commands

## Related

- USER_GUIDE.md section 2.3: "You should see log entries...the echo plugin executing"
- CLI_DESIGN_PRINCIPLES.md: Verbose flag for internal logic
- TP-001: First test case that discovered this issue

## Narrative

- 2026-02-13: Discovered during TP-001 (Schedule Trigger UAT). Gateway started successfully and echo plugin configured with 5m schedule. Startup logs appeared normally (19 lines), showing scheduler enqueue 3 poll jobs. However, no further logs appeared despite jobs executing successfully (confirmed via database query). All jobs completed with status=succeeded and plugin logs stored in job_log table. This makes real-time monitoring impossible and contradicts USER_GUIDE.md which states execution logs should be visible. Hypothesis: dispatch component logs at DEBUG level, filtered by INFO threshold. (by @test-admin)
