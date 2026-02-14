# Bug #94: Monitor TUI Not Displaying Events

**Status**: backlog
**Priority**: medium
**Component**: tui/monitor
**Introduced**: PR #38 (TUI monitor feature)
**Affected Version**: 0.1.0-dev (commit 39c693e)

## Summary

The `ductile system monitor` TUI launches successfully and connects to the events endpoint, but does not display any real-time activity in the interface. Events are being broadcast correctly via SSE, but the TUI rendering is not showing them.

## Reproduction

1. Start gateway: `ductile system start --config config.yaml`
2. Launch monitor: `ductile system monitor --api-url http://localhost:8080 --api-key <token>`
3. Trigger a job: `curl -X POST http://localhost:8080/trigger/echo/poll`
4. **Expected**: TUI shows job.started, plugin.spawned, job.completed events
5. **Actual**: TUI displays interface but shows no activity

## Evidence

**Gateway logs confirm events are broadcast:**
```
{"level":"INFO","msg":"http request","method":"GET","path":"/events","status":200}
{"level":"INFO","msg":"job started","job_id":"3fb5eaed...","plugin":"echo"}
{"level":"INFO","msg":"job completed","job_id":"3fb5eaed...","status":"succeeded"}
```

**Manual SSE test confirms events streaming:**
```bash
curl -N -H "Authorization: Bearer <token>" http://localhost:8080/events
# Returns:
event: job.started
data: {"job_id":"...","plugin":"echo"}

event: job.completed
data: {"job_id":"...","status":"succeeded"}
```

**Monitor connects but doesn't render:**
- TUI interface visible
- Monitor process running (confirmed via ps)
- `/events` endpoint shows 200 status (connected)
- No activity displayed in UI

## Analysis

**Working components:**
- ✅ Event broadcasting infrastructure (`internal/events/hub.go`)
- ✅ SSE endpoint (`/events`) streaming correctly
- ✅ Monitor connects to endpoint (GET /events returns 200)
- ✅ Events contain correct data

**Suspected issue:**
- ❌ TUI client-side event rendering (`internal/tui/monitor.go`)
- Possible buffering issue
- Event format mismatch between hub and TUI
- BubbleTea message handling not processing SSE events

## Investigation Needed

1. **Check SSE client implementation** in `monitor.go`:
   - Is the HTTP client properly reading the SSE stream?
   - Are events being parsed correctly from SSE format?
   - Are BubbleTea messages being sent on event receipt?

2. **Verify event format compatibility**:
   - Compare event format emitted by hub vs expected by TUI
   - Check if event IDs, types, or data structure differs

3. **Test BubbleTea update cycle**:
   - Add debug logging to Update() method
   - Verify messages are reaching the TUI
   - Check if View() is being called after updates

4. **Buffer flushing**:
   - SSE streams may be buffered
   - Verify io.Reader is not blocking
   - Check if events arrive but aren't processed in real-time

## Files to Review

- `internal/tui/monitor.go` (424 lines) - TUI implementation
- `internal/events/hub.go` - Event broadcasting
- `internal/api/events_handler.go` - SSE endpoint
- `cmd/ductile/main.go` - Monitor command setup

## Workaround

Manual event monitoring via curl:
```bash
curl -N -H "Authorization: Bearer test_admin_token_local" \
  http://localhost:8080/events
```

## Related

- PR #38: feat: implement monitoring TUI with real-time event broadcasting
- Uses: bubbletea, bubbles, lipgloss (Charm TUI libraries)

## Testing Context

- Environment: `/home/matt/admin/ductile-test/`
- Binary: ductile 0.1.0-dev (39c693e1f492)
- All 6 plugins operational
- API server running on localhost:8080
- Event streaming confirmed working

**Discovered**: 2026-02-14
**Tester**: @matt
**Session**: RFC-92 regression testing + plugin validation
