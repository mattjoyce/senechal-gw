---
id: 87
status: todo
priority: High
tags: [bug, pipeline, routing, events, orchestration]
---

# BUG: Pipeline routing not triggering child jobs

## Description

Plugins emit events successfully but the pipeline router doesn't create child jobs. Events are captured in job logs but no routing/orchestration occurs.

## Impact

- **Severity**: High - Blocks multi-hop pipeline functionality
- **Scope**: All event-driven pipelines
- **User Experience**: Single-step execution only, no workflow orchestration

## Evidence

**Test**: TP-002 Pipeline Routing (2026-02-13)

**Setup**:
- Pipeline configured: `file-to-report` (file.read → fabric → file_handler)
- Triggered via API: `/trigger/file_handler/handle`
- Payload: read sample.md file

**Expected flow**:
1. file_handler reads file → emits `file.read` event
2. Router matches event to pipeline
3. fabric step executes (analyze)
4. file_handler step executes (save report)

**Actual behavior**:
1. ✅ file_handler reads file successfully
2. ✅ Event emitted: `file.read` with full payload
3. ❌ **No routing occurs** - no child jobs created
4. ❌ Pipeline never executes

**Job log evidence**:
```json
{
  "status": "ok",
  "events": [{
    "type": "file.read",
    "payload": {
      "file_path": "/home/matt/admin/ductile-test/test-files/sample.md",
      "filename": "sample.md",
      "content": "...",
      "size_bytes": 484
    }
  }],
  "logs": [{"level": "info", "message": "Read 484 bytes from sample.md"}]
}
```

**Database check**:
```sql
-- Job completed but no children
SELECT parent_job_id, event_context_id
FROM job_queue
WHERE id='fbd8eac4-cdb3-494c-8416-cd64a77941d2';
-- Result: NULL, NULL
```

## Root Cause (Suspected)

**Hypothesis 1**: Router/dispatcher not watching for events
- Events emitted and stored in job_log
- But no routing logic triggered
- Dispatcher may not be checking job completion for events

**Hypothesis 2**: Pipeline config not loaded properly
- pipelines.yaml included in config
- But routing table may not be initialized
- No startup logs showing pipeline discovery

**Hypothesis 3**: Feature not fully implemented
- Code exists: `internal/dispatch/dispatcher_test.go` shows routing tests
- But may not be wired up in main.go
- Missing initialization of router component?

## Code References

**Evidence routing exists**:
- `/home/matt/ductile/internal/dispatch/dispatcher_test.go`:
  - `TestDispatcher_RoutesTwoHopChainWithContextAndWorkspace`
- `/home/matt/ductile/internal/config/validator.go`:
  - `validateRoutes()` function exists

**Missing from logs**:
- No "pipeline loaded" messages
- No "routing event" messages
- No "matched pipeline" messages

## Reproduction

```bash
# Start gateway
./ductile system start --config config.yaml

# Trigger file read
curl -X POST http://localhost:8080/trigger/file_handler/handle \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"payload": {"action": "read", "file_path": "/path/to/file.md"}}'

# Check for child jobs (none will exist)
sqlite3 data/state.db "
  SELECT id, plugin, parent_job_id
  FROM job_queue
  WHERE created_at > datetime('now', '-1 minute')
  ORDER BY created_at;
"
# Expected: 3 jobs (initial + 2 pipeline steps)
# Actual: 1 job (initial only)
```

## Testing Recommendations

1. **Check dispatcher initialization**:
   - Does main.go create/start a router component?
   - Are pipelines passed to dispatcher?

2. **Add router logging**:
   - Log when events are checked
   - Log when pipelines are matched
   - Log when child jobs are enqueued

3. **Verify pipeline loading**:
   - Add startup log for pipeline discovery
   - Validate pipelines load into routing table

**After fix, verify**:
- [ ] Pipeline routes file.read → fabric
- [ ] fabric job has parent_job_id set
- [ ] fabric job has event_context_id
- [ ] Chain completes: file_handler → fabric → file_handler
- [ ] Report generated in output directory

## Related

- **TP-002**: Test plan for pipeline routing
- **Bug #80**: job inspect --json (blocked by this bug - need pipelines for lineage)
- **PIPELINES.md**: Documents expected behavior

## Workaround

None - manual triggering of each step defeats purpose of pipelines.

## Narrative

- 2026-02-13: Discovered during TP-002 (Pipeline Routing UAT). Configured file-to-report pipeline and triggered file_handler to read sample.md. Plugin successfully read file and emitted file.read event with full payload (484 bytes, verified in job_log). However, router did not create any child jobs. Database shows NULL for parent_job_id and event_context_id. No routing logs appeared. Code review shows routing tests exist in dispatcher_test.go, suggesting feature is partially implemented but not wired up. This blocks all multi-hop pipeline functionality and Bug #80 testing (needs lineage/context from pipelines). (by @test-admin)
