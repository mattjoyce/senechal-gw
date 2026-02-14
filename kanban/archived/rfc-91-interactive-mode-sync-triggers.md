---
id: 91
status: backlog
priority: High
blocked_by: []
tags: [architecture, api, discord, webhooks, interactive]
---

# RFC: Interactive Mode / Synchronous Triggers

## Problem Statement

ductile's async-only architecture is **incompatible with interactive use cases** (chat bots, web UIs, CLI tools that expect immediate responses).

### Current Behavior
All `/trigger/*` endpoints return HTTP 202 with job ID immediately. Jobs run async in background. **No mechanism exists to retrieve or push results back to caller.**

### What This Breaks
- **Discord/Slack bots**: User sends "/ai summarize this", bot shows "queued" but never delivers the actual summary
- **Web APIs**: Frontend calls gateway, gets "queued", has no way to get the actual result
- **CLI wrappers**: Scripts that need to wait for output and display it
- **Any request/response pattern**: Fundamentally mismatched with fire-and-forget design

### Evidence
**Test case**: Discord bot → fabric plugin
- Job succeeds (ID: `a25b1490-4cd3-4d6d-a44d-4685e22a63b7`)
- User receives: "**queued:** No message provided"
- Actual fabric output never reaches user
- See: `/home/matt/admin/ductile-test/lab-notes.md` (line 569+)

## Root Cause

ductile is architected for **batch processing**:
- Optimized for multi-hop pipelines, file processing, automation
- "Fire and forget" model with workspace dirs and audit trails
- No result delivery mechanism (webhooks, polling endpoints, or sync mode)

This is a **design choice**, not a bug. But it creates a hard limitation.

## Proposed Solutions

### Option 1: Client-Side Polling (Band-aid)
**Implementation**: Clients poll `GET /job/{id}/status` until complete, then fetch result

**Pros**:
- No gateway changes needed
- Works with existing async model

**Cons**:
- Requires new `/job/{id}/result` endpoint anyway
- Adds latency (polling interval)
- Wastes resources (repeated requests)
- Complex client logic

### Option 2: Webhook Callbacks (Push model)
**Implementation**: Accept `callback_url` in trigger request, POST results when complete

```yaml
POST /trigger/fabric/handle
{
  "payload": {"text": "hello"},
  "callback_url": "https://discord.webhook/..."
}
```

**Pros**:
- Efficient (no polling)
- Gateway remains async
- Works for long-running jobs

**Cons**:
- Requires callback handling infrastructure (Discord webhook, HTTP endpoints)
- Security concerns (SSRF, auth)
- Doesn't help CLI/terminal use cases

### Option 3: Synchronous Mode Flag (Recommended)
**Implementation**: Add `?wait=true` query param or `sync: true` field. Gateway blocks until job completes, returns result in response.

```bash
POST /trigger/fabric/handle?wait=true
# Returns after job completes:
{
  "job_id": "...",
  "status": "succeeded",
  "result": {
    "stdout": "...",
    "output_files": [...]
  }
}
```

**Pros**:
- Simple for clients (standard HTTP request/response)
- Works for chat bots, web UIs, CLI tools
- Gateway still uses same async job system internally
- Optional (existing async mode unchanged)

**Cons**:
- Ties up HTTP connection during execution
- Needs timeout handling (e.g., 30s max, return "still running" status)
- Needs request context cancellation if client disconnects

### Option 4: SSE/WebSocket Streaming (Future)
**Implementation**: Upgrade connection, stream results in real-time

**Pros**:
- Best UX (see output as it happens)
- Works for long-running jobs

**Cons**:
- High complexity
- Major protocol change
- Overkill for simple use cases

### Option 5: Separate "ductile-sync" Service (Out of scope?)
**Implementation**: Different service for sync operations, delegates to ductile

**Pros**:
- Keeps ductile focused on async workflows
- Clean separation of concerns

**Cons**:
- More services to maintain
- Duplicated plugin logic
- Doesn't solve the core problem

## Recommendation

**Implement Option 3: Synchronous Mode Flag**

Add `?wait=true` support to `/trigger` endpoints:
1. Job still queued normally (same code path)
2. If `wait=true`, dispatcher notifies caller when job completes
3. Response includes job result (stdout, output files, status)
4. Use timeout (30s default) to prevent hung connections
5. Return partial status if timeout exceeded: `{"status": "running", "job_id": "..."}`

This solves the immediate problem (Discord bot, interactive use) while preserving async architecture.

## Implementation Sketch

```go
// api/handlers.go
func (h *Handler) Trigger(w http.ResponseWriter, r *http.Request) {
    // ... existing job queue logic ...

    waitForResult := r.URL.Query().Get("wait") == "true"

    if !waitForResult {
        // Existing behavior: return 202 immediately
        respondJSON(w, 202, QueuedResponse{JobID: jobID, Status: "queued"})
        return
    }

    // NEW: Wait for completion
    result, err := h.dispatcher.WaitForJob(r.Context(), jobID, 30*time.Second)
    if err == context.DeadlineExceeded {
        respondJSON(w, 202, RunningResponse{JobID: jobID, Status: "running"})
        return
    }

    respondJSON(w, 200, JobResult{
        JobID: jobID,
        Status: result.Status,
        Stdout: result.Stdout,
        OutputFiles: result.OutputFiles,
    })
}
```

## Open Questions

1. Should sync mode be supported for pipeline triggers, or only single plugins?
2. What's the right timeout? Configurable per-plugin?
3. How to stream large outputs (e.g., fabric summary of 10-page doc)?
4. Should we add `/job/{id}/result` endpoint regardless (for manual polling)?

## Success Criteria

- [ ] Discord bot receives actual fabric output instead of "queued"
- [ ] Web UI can call gateway and display results synchronously
- [ ] CLI tools can use `--wait` flag for blocking behavior
- [ ] Existing async clients unaffected (backward compatible)
- [ ] Performance acceptable (no worse than plugin execution time + small overhead)

## Update: See RFC-92

**RFC-92 (Pipeline Execution Modes)** proposes a better solution than the query param approach discussed here. Instead of HTTP-level `?wait=true`, make execution semantics a declarative pipeline concern using `execution_mode: synchronous` in config.

**Advantages of RFC-92**:
- Reuses existing DAG infrastructure
- Declarative (YAML config) vs imperative (query params)
- Works with complex pipelines (`split`, `call`, `on_events`)
- Per-pipeline timeout control
- Aligns with ductile design philosophy

This RFC (91) remains valuable for documenting the problem and exploring alternatives.

## Narrative
- 2026-02-13: Created after discovering Discord bot integration returns "queued" but never delivers results. Root cause is architectural: gateway is async-only, but interactive use cases need sync responses. (by @assistant)
- 2026-02-13: Superseded by RFC-92 which proposes pipeline-level execution modes as more elegant solution. (by @assistant)
