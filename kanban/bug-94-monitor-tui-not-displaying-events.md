# Bug #94: Monitor TUI Not Displaying Events

**Status**: done
**Priority**: medium
**Resolution**: Fixed in commit 4399202
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

## Root Cause Analysis

### Bug #1: SSE Parsing Error (Lines 383-390)

**Current code**:
```go
for scanner.Scan() {
    line := scanner.Text()
    if strings.HasPrefix(line, "data: ") {
        var ev events.Event
        if err := json.Unmarshal([]byte(line[6:]), &ev); err == nil {
            m.hubEvents <- ev
        }
    }
}
```

**Problem**: SSE events are multi-line:
```
id: 123
event: job.started
data: {"job_id":"...","plugin":"..."}

```

The code only reads `data:` lines and tries to unmarshal `{"job_id":"..."}` as an `Event{ID, Type, At, Data}` struct. This fails silently because the JSON payload doesn't contain `id`, `type`, or `at` fields - it only contains the event-specific data.

**Fix**: Parse all three SSE fields (id, event, data) and construct the complete Event struct.

### Bug #2: Event Loop Never Starts (Init function)

**Current code** (lines 125-131):
```go
func (m Model) Init() tea.Cmd {
    return tea.Batch(
        m.subscribeToEvents(),  // Starts SSE goroutine
        m.pollHealth(),
        tea.EnterAltScreen,
    )
}
```

**Problem**: `subscribeToEvents()` starts reading events and sending to `m.hubEvents` channel, but nothing reads from the channel! The `receiveNextEvent()` function only gets called in `Update()` when an `eventMsg` arrives (line 155), creating a chicken-and-egg problem:
- No initial call to `receiveNextEvent()` → no one reading from channel
- Events pile up in channel buffer (100 capacity)
- No `eventMsg` ever reaches `Update()` to trigger the loop

**Fix**: Add `m.receiveNextEvent()` to the `Init()` batch to kickstart the event consumption loop.

## Proposed Fix

### Fix #1: Correct SSE Parsing

Replace lines 383-390 in `monitor.go` with proper multi-line SSE parser:

```go
var currentEvent struct {
    id    int64
    typ   string
    data  string
}

for scanner.Scan() {
    line := scanner.Text()

    if line == "" {
        // Empty line marks end of event
        if currentEvent.data != "" {
            ev := events.Event{
                ID:   currentEvent.id,
                Type: currentEvent.typ,
                At:   time.Now(), // Or parse from data if available
                Data: []byte(currentEvent.data),
            }
            m.hubEvents <- ev
            currentEvent = struct{ id int64; typ string; data string }{}
        }
        continue
    }

    if strings.HasPrefix(line, "id: ") {
        currentEvent.id, _ = strconv.ParseInt(line[4:], 10, 64)
    } else if strings.HasPrefix(line, "event: ") {
        currentEvent.typ = line[7:]
    } else if strings.HasPrefix(line, "data: ") {
        currentEvent.data = line[6:]
    }
}
```

### Fix #2: Kickstart Event Loop

Add `receiveNextEvent()` call in `Init()` function (line 125):

```go
func (m Model) Init() tea.Cmd {
    return tea.Batch(
        m.subscribeToEvents(),
        m.receiveNextEvent(),  // ← ADD THIS LINE
        m.pollHealth(),
        tea.EnterAltScreen,
    )
}
```

## Files to Review

- `internal/tui/monitor.go` (424 lines) - **TUI implementation (needs both fixes)**
- `internal/events/hub.go` - Event broadcasting (working)
- `internal/api/events_handler.go` - SSE endpoint (working)
- `cmd/ductile/main.go` - Monitor command setup (working)

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

## Resolution

**Fixed**: 2026-02-14  
**Commit**: `4399202` - Fix Bug #94: Monitor TUI not displaying events

### Changes Made

**File**: `internal/tui/monitor.go`

**Fix #1: SSE Parser** (39 lines added, 3 removed)
- Replaced single-line parser with multi-line SSE parser
- Parse id, event, and data fields separately
- Construct complete Event struct with all fields
- Handle empty line as event delimiter

**Fix #2: Event Loop** (1 line added)
- Added `m.receiveNextEvent()` to `Init()` batch
- Kickstarts event consumption from channel

**Import Added**:
- `strconv` for ParseInt

### Testing
- Binary rebuilt with fixes
- Gateway restarted  
- Jobs triggered: echo, fabric, jina-reader
- All jobs executed successfully
- Monitor now displays events in real-time

**Status**: ✅ Fixed and deployed
