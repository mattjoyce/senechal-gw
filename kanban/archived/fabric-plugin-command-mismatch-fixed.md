---
id: 66
status: done
priority: Normal
blocked_by: []
tags: [bug, plugin, fabric, fixed]
---

# BUG: Fabric Plugin Command Mismatch - Fixed

## Description

The fabric plugin's `run.py` implementation used "execute" command which doesn't exist in the gateway protocol, while the manifest declared "poll" and "health". This caused runtime failures when the plugin was triggered.

## Error

```
{"level":"WARN","msg":"plugin returned error","plugin":"fabric","command":"poll",
 "error":"Unknown command: poll"}
```

## Root Cause

Mismatch between manifest declaration and implementation:
- **Manifest**: Declared `commands: [poll, health]`
- **Implementation**: Only handled `execute` and `health` commands

The plugin code had `execute_command()` function but no `poll_command()` or `handle_command()`.

## Resolution

Updated `plugins/fabric/run.py` to implement proper protocol commands:

1. **Renamed and refactored**:
   - `execute_command()` → `handle_command()` (for event-driven execution)
   - Added new `poll_command()` (for scheduled execution)

2. **Updated command routing**:
```python
if command == "poll":
    response = poll_command(config, state)
elif command == "handle":
    response = handle_command(config, state, event)
elif command == "health":
    response = health_command(config)
```

3. **Updated manifest** to include all supported commands:
```yaml
commands: [poll, handle, health]
```

## Implementation Details

- **poll**: Returns success with timestamp (no-op, for future scheduled actions)
- **handle**: Processes events by calling fabric CLI with pattern
- **health**: Checks fabric binary availability and pattern count

## Verification

```bash
curl -X POST http://localhost:8080/trigger/fabric/poll \
  -H "Authorization: Bearer $TOKEN"
```

**Result**: ✅ Success
```json
{
  "status": "succeeded",
  "result": {
    "status": "ok",
    "state_updates": {"last_poll": "2026-02-11T22:03:56Z"},
    "logs": [{"level": "info", "message": "Fabric poll command - no scheduled actions configured"}]
  }
}
```

## Testing Status

- ✅ Poll command works
- ⏳ Handle command not tested (requires event payload)
- ⏳ Health command not tested

## Narrative

- 2026-02-12: Discovered after fixing manifest - plugin loaded successfully but failed at runtime when triggered. The run.py only handled "execute" command which isn't part of the v1 protocol. Fixed by implementing poll_command() and renaming execute_command() to handle_command() to align with protocol specification. Poll now works as a no-op placeholder for future scheduled actions. The handle command contains the actual fabric CLI execution logic. (by @assistant/test-admin)
