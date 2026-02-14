---
id: 87
status: todo
priority: High
blocked_by: []
tags: [bug, api, plugin, core, handle-command]
---

# BUG: Plugin fails when triggered via API but works manually

## Description

The `file_handler` plugin (and potentially other handle-based plugins) fail when triggered via `POST /trigger/{plugin}/handle` but work correctly when tested manually with identical JSON input.

## Impact

- **Severity**: High - Blocks E2E pipeline testing (card #62)
- **Scope**: API trigger mechanism, affects event-driven plugins
- **Workaround**: None for API-triggered handle commands

## Evidence

### Manual Test (SUCCESS ✅)
```bash
docker exec ductile-test bash -c 'cat /tmp/request.json | python3 /app/plugins/file_handler/run.py'
```
**Result**: Plugin executes successfully, returns valid JSON response with status "ok", emits events

### API Trigger (FAILURE ❌)
```bash
curl -X POST http://localhost:8080/trigger/file_handler/handle \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"payload": {"action": "read", "file_path": "/tmp/sample.md"}}'
```
**Result**:
- Exit code: 1
- stdout: empty (no output)
- Gateway error: `"plugin produced no output on stdout"`
- No stderr captured in logs

## Reproduction

**Test Plugin:**
```python
# plugins/file_handler/run.py
# Pythonic, type-hinted, comprehensive error handling
# Works perfectly when invoked manually
```

**Test Input (works manually):**
```json
{
  "command": "handle",
  "config": {"allowed_read_paths": "/tmp"},
  "state": {},
  "event": {
    "payload": {
      "action": "read",
      "file_path": "/tmp/sample.md"
    }
  }
}
```

**API Trigger (fails):**
```bash
POST /trigger/file_handler/handle
Body: {"payload": {"action": "read", "file_path": "/tmp/sample.md"}}
```

## Observed Behavior

```
{"level":"INFO","msg":"executing job","job_id":"...","plugin":"file_handler","command":"handle"}
{"level":"DEBUG","msg":"spawning plugin","entrypoint":"/app/plugins/file_handler/run.py"}
{"level":"WARN","msg":"plugin exited with non-zero status","exit_code":1}
{"level":"ERROR","msg":"failed to decode plugin response","error":"plugin produced no output on stdout","stdout":""}
```

## Analysis

### Possible Root Causes

1. **Event Envelope Construction**
   - API may construct the request envelope differently
   - Possible missing/malformed `event` field
   - JSON encoding issue

2. **stdin/stdout Handling**
   - API subprocess spawn may have different stdio configuration
   - Buffer flush timing issue
   - Encoding mismatch (UTF-8 vs ASCII)

3. **Python Process Environment**
   - Different environment variables between manual exec and API spawn
   - Python path or module import issues
   - Working directory mismatch

4. **JSON Parsing**
   - Plugin receives malformed JSON from API
   - Extra/missing fields in request envelope
   - Character encoding issue

### Why Manual Test Works

The manual test uses identical JSON structure and succeeds, which suggests:
- Plugin code is correct
- JSON parsing logic works
- Event handling logic works
- The issue is in how the gateway constructs/sends the request

## Debug Steps Needed

1. **Capture stderr** - Gateway currently doesn't log Python tracebacks
2. **Log request envelope** - What exactly does the gateway send to the plugin?
3. **Compare requests** - Manual vs API: are they truly identical?
4. **Add debug logging** - Log raw stdin received by plugin
5. **Test with simple plugin** - Does `echo` plugin work via handle command?

## Temporary Workarounds

None available. API triggering of handle commands is non-functional.

## Test Environment

- Gateway: Latest main branch
- Plugin: file_handler v0.1.0 (Pythonic exemplar)
- Container: ductile-test (Docker)
- API: http://localhost:8080
- Auth: Token-based (working for poll triggers)

## Related

- Card #62: Multi-Plugin Pipeline E2E Test (BLOCKED by this issue)
- Card #64: API Authentication Test (PASS - poll commands work)

## Narrative

- 2026-02-12: Discovered during E2E pipeline testing for card #62. The file_handler plugin was created as a Pythonic exemplar with comprehensive error handling, type hints, and security validation. Plugin works perfectly when tested manually via `docker exec` with identical JSON input. However, when triggered via POST /trigger/file_handler/handle, it consistently exits with code 1 and produces no stdout output. The gateway reports "plugin produced no output on stdout". No Python traceback is captured because stderr logging is not implemented. This blocks all event-driven plugin testing via API. Needs core team investigation into API request envelope construction and subprocess stdio handling. (by @test-admin)
- 2026-02-13: Renumbered from card #68 to card #87 to resolve duplicate kanban ID collision with the separate completed API payload-envelope bug card. (by @assistant)
