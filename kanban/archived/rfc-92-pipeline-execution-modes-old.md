---
id: 92
status: backlog
priority: High
blocked_by: []
tags: [architecture, pipelines, interactive, dag, critical]
---

# RFC: Pipeline Execution Modes (Declarative Sync/Async)

## Executive Summary

**Problem**: ductile's async-only architecture blocks interactive use cases (Discord bots, web UIs, CLI tools). See RFC-91 for full problem statement.

**Proposed Solution**: Add `execution_mode` to pipeline config, leveraging existing DAG infrastructure to make pipelines wait for completion before returning results.

**Why This Saves the Project**: Instead of bolting sync behavior onto HTTP endpoints (hacky), we make execution semantics a **first-class pipeline concern**. This is architecturally sound and reuses existing dispatcher parent-child tracking.

## Design Philosophy

### Core Insight

ductile already has all the infrastructure needed:
- ✓ Dispatcher tracks parent-child job relationships
- ✓ Pipelines define multi-step workflows with `split`, `call`, `on_events`
- ✓ Job completion is already tracked in database
- ✓ Results are already captured (stdout, workspace artifacts)

**Missing piece**: A way to declare "this pipeline should wait for its entire execution tree before returning control to the caller."

### Declarative > Imperative

Rather than making clients say `?wait=true` on every request, let the **pipeline author** declare execution semantics:

```yaml
pipelines:
  - name: interactive-fabric
    on: fabric.handle
    execution_mode: synchronous  # Author declares: "This is interactive"
    timeout: 30s
    steps:
      - uses: fabric
```

This is **better than HTTP-level flags** because:
1. Config documents pipeline behavior
2. Client doesn't need to know implementation details
3. Works naturally with complex multi-step workflows
4. Aligns with ductile's "YAML-configured" design philosophy

## Proposed Schema

### Pipeline-Level Execution Mode

```yaml
pipelines:
  - name: <pipeline-name>
    on: <event-trigger>
    execution_mode: synchronous | async  # NEW field
    timeout: <duration>  # NEW: Max wait time (e.g., "30s", "2m")
    return_on_timeout: partial | job_id  # NEW: Behavior when timeout exceeded
    steps:
      - uses: <plugin>
```

### Step-Level Wait Control (Future Extension)

```yaml
steps:
  - id: main
    uses: fabric
    wait_for_children: true  # NEW: Block until downstream steps complete

  - uses: formatter
    # Runs after 'main' completes (because main waits)
```

## Execution Modes

### Mode 1: `async` (Default, Current Behavior)

```yaml
execution_mode: async
```

**Behavior**:
- Trigger endpoint returns HTTP 202 immediately
- Job queued and runs in background
- No result returned to caller
- **Use for**: Fire-and-forget workflows, batch processing, long-running pipelines

### Mode 2: `synchronous` (New)

```yaml
execution_mode: synchronous
timeout: 30s
```

**Behavior**:
- Trigger endpoint **blocks** until pipeline completes (or timeout)
- Dispatcher waits for root job + all descendant jobs (entire DAG subtree)
- Returns HTTP 200 with results when complete
- Returns HTTP 202 with partial status if timeout exceeded
- **Use for**: Interactive workflows, chat bots, web APIs, CLI tools

## API Response Format

### Synchronous Pipeline (Success)

```json
POST /trigger/fabric/handle

Response: HTTP 200 OK
{
  "job_id": "a25b1490-...",
  "status": "succeeded",
  "execution_mode": "synchronous",
  "duration_ms": 1250,
  "result": {
    "stdout": "SUMMARY:\n- Key point 1\n- Key point 2",
    "stderr": "",
    "exit_code": 0,
    "output_files": [
      {
        "path": "summary.md",
        "size": 512,
        "url": "/workspace/a25b1490-.../summary.md"
      }
    ]
  },
  "children": [
    {
      "job_id": "child-1-...",
      "plugin": "formatter",
      "status": "succeeded"
    }
  ]
}
```

### Synchronous Pipeline (Timeout)

```json
Response: HTTP 202 Accepted
{
  "job_id": "a25b1490-...",
  "status": "running",
  "execution_mode": "synchronous",
  "timeout_exceeded": true,
  "timeout_ms": 30000,
  "elapsed_ms": 30001,
  "message": "Pipeline still running after 30s timeout. Check /job/{id} for status."
}
```

### Async Pipeline (Current Behavior)

```json
Response: HTTP 202 Accepted
{
  "job_id": "a25b1490-...",
  "status": "queued",
  "execution_mode": "async",
  "plugin": "fabric",
  "command": "handle"
}
```

## Implementation Plan

### Phase 1: Core Infrastructure (Week 1)

**Files to modify**:
- `internal/config/types.go`: Add `ExecutionMode`, `Timeout`, `ReturnOnTimeout` fields to `Pipeline` struct
- `internal/config/loader.go`: Parse new fields from YAML
- `internal/dispatch/dispatcher.go`: Add `WaitForCompletion(jobID, timeout)` method

**WaitForCompletion logic**:
```go
func (d *Dispatcher) WaitForCompletion(ctx context.Context, jobID string, timeout time.Duration) (*JobResult, error) {
    deadline := time.After(timeout)
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-deadline:
            return nil, ErrTimeout
        case <-ticker.C:
            // Check if root job + all descendants complete
            complete, result := d.checkJobTreeComplete(jobID)
            if complete {
                return result, nil
            }
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
}

func (d *Dispatcher) checkJobTreeComplete(rootID string) (bool, *JobResult) {
    // Query database for root job + all jobs with parent_job_id ancestry
    // Return true only if ALL jobs in tree have status in (succeeded, failed)
    // Aggregate results from all jobs in tree
}
```

### Phase 2: API Handler Integration (Week 1)

**Files to modify**:
- `cmd/ductile/api/handlers.go`: Update trigger handlers

```go
func (h *Handler) TriggerPlugin(w http.ResponseWriter, r *http.Request) {
    // ... existing job queue logic ...

    // NEW: Check if pipeline has execution_mode = synchronous
    pipeline := h.router.GetPipeline(pluginName, commandName)

    if pipeline != nil && pipeline.ExecutionMode == config.ExecutionModeSync {
        // Synchronous path
        timeout := pipeline.Timeout
        if timeout == 0 {
            timeout = 30 * time.Second  // Default
        }

        result, err := h.dispatcher.WaitForCompletion(r.Context(), jobID, timeout)

        if err == ErrTimeout {
            // Return partial status
            w.WriteHeader(http.StatusAccepted)
            json.NewEncoder(w).Encode(TimeoutResponse{
                JobID:           jobID,
                Status:          "running",
                ExecutionMode:   "synchronous",
                TimeoutExceeded: true,
                TimeoutMs:       int(timeout.Milliseconds()),
            })
            return
        }

        // Return complete result
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(SyncResponse{
            JobID:         jobID,
            Status:        result.Status,
            ExecutionMode: "synchronous",
            DurationMs:    result.Duration.Milliseconds(),
            Result:        result,
            Children:      result.Children,
        })
        return
    }

    // Existing async path
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(AsyncResponse{
        JobID:         jobID,
        Status:        "queued",
        ExecutionMode: "async",
    })
}
```

### Phase 3: Result Aggregation (Week 2)

**Challenge**: For pipelines with `split` or multiple steps, aggregate results from all child jobs.

**Approach**:
```go
type JobResult struct {
    JobID      string
    Plugin     string
    Command    string
    Status     string
    ExitCode   int
    Stdout     string
    Stderr     string
    Duration   time.Duration
    OutputFiles []FileInfo
    Children   []JobResult  // Recursive: results from child jobs
}

func (d *Dispatcher) aggregateResults(rootJobID string) (*JobResult, error) {
    // Recursively fetch root job + all descendants
    // Build tree structure matching pipeline DAG
    // Include stdout, artifacts, status for each node
}
```

### Phase 4: Timeout Handling (Week 2)

**Design decision**: What happens when timeout is exceeded?

**Option A**: Kill running jobs (aggressive)
```yaml
execution_mode: synchronous
timeout: 30s
return_on_timeout: partial
on_timeout: kill  # NEW: Cancel running jobs
```

**Option B**: Let jobs continue, return job_id (recommended)
```yaml
execution_mode: synchronous
timeout: 30s
return_on_timeout: job_id  # Return tracking info, jobs keep running
```

**Recommendation**: Option B. Don't kill jobs just because client timed out. Client can poll `/job/{id}` if needed.

### Phase 5: Testing (Week 3)

**Test cases**:
1. Simple sync pipeline (single step)
2. Multi-step sequential sync pipeline
3. Sync pipeline with `split` (parallel branches)
4. Sync pipeline with `call` (nested pipelines)
5. Sync pipeline timeout (partial return)
6. Mixed: sync pipeline calling async pipeline
7. Discord integration test (end-to-end)

## Discord Use Case (Solved)

### Pipeline Config

```yaml
# pipelines/interactive-fabric.yaml
pipelines:
  - name: discord-fabric
    on: fabric.handle
    execution_mode: synchronous
    timeout: 30s
    steps:
      - uses: fabric
```

### Discord Bot (No Changes Needed!)

```python
# Bot code stays the same
response = requests.post(
    "http://localhost:8080/trigger/fabric/handle",
    json={"payload": {"text": user_message, "pattern": "summarize"}}
)

# Now response.json() contains actual result:
result = response.json()
if result["status"] == "succeeded":
    await message.channel.send(result["result"]["stdout"])
else:
    await message.channel.send(f"Error: {result['status']}")
```

**Bot receives actual fabric output instead of "queued"!** ✓

## Edge Cases & Open Questions

### Q1: What about single-plugin triggers (no pipeline)?

**Option A**: Default to async (current behavior)
```bash
POST /trigger/fabric/handle  # No pipeline exists
→ Returns 202 "queued" (async)
```

**Option B**: Allow plugins to declare execution mode in manifest
```yaml
# plugins/fabric/manifest.yaml
plugin: fabric
execution_mode: synchronous  # NEW: Plugin-level default
timeout: 30s
commands:
  - handle
```

**Recommendation**: Option B for flexibility, but pipelines override plugin defaults.

### Q2: Nested pipelines with different modes?

```yaml
# Pipeline A: synchronous
- name: parent
  execution_mode: synchronous
  steps:
    - call: child-pipeline  # child-pipeline is async

# What happens?
```

**Answer**: Parent waits for child pipeline's root job to complete, but not child's descendants. Parent's `synchronous` mode only waits for its direct children.

**Alternative**: `execution_mode: deep_synchronous` waits for entire call tree (expensive).

### Q3: How to handle `split` with partial failures?

```yaml
steps:
  - split:
      - uses: branch-a  # succeeds
      - uses: branch-b  # fails
      - uses: branch-c  # succeeds
```

**Answer**: Return all results with their individual statuses. Overall status = "partial_success" if any branch failed.

```json
{
  "status": "partial_success",
  "children": [
    {"plugin": "branch-a", "status": "succeeded"},
    {"plugin": "branch-b", "status": "failed", "error": "..."},
    {"plugin": "branch-c", "status": "succeeded"}
  ]
}
```

### Q4: Security concern - DoS via long-running sync pipelines?

**Mitigation**:
1. Enforce max timeout (e.g., 5 minutes)
2. Rate limit sync trigger endpoints
3. Separate HTTP server pool for sync vs async?

```yaml
api:
  sync_max_timeout: 5m
  sync_max_concurrent: 10  # Limit concurrent blocking requests
```

## Benefits Over HTTP-Level `?wait=true`

| Aspect | Pipeline `execution_mode` | Query Param `?wait=true` |
|--------|--------------------------|--------------------------|
| **Discoverability** | Config documents behavior | Client must know to use it |
| **Control** | Author declares intent | Client decides (chaos) |
| **Per-pipeline semantics** | Natural (each pipeline is different) | One-size-fits-all |
| **Complex workflows** | Works with split/call/events | Doesn't understand DAG |
| **Timeout control** | Per-pipeline (30s for chat, 5m for reports) | Global or per-request |
| **Aligns with design** | YAML-configured, declarative | HTTP-level imperative |

## Migration Path

### Phase 1: Add feature (no breaking changes)
- All existing pipelines default to `execution_mode: async`
- New pipelines opt-in to `synchronous`
- Clients unchanged (still get 202 for async pipelines)

### Phase 2: Migrate interactive pipelines
```yaml
# Before
pipelines:
  - name: fabric-analyze
    on: fabric.handle

# After
pipelines:
  - name: fabric-analyze
    on: fabric.handle
    execution_mode: synchronous  # Now returns results!
    timeout: 30s
```

### Phase 3: Update clients to use results
- Discord bot: Read `result.stdout` from response
- Web UI: Display results immediately
- CLI tools: Print output and exit

## Success Criteria

- [ ] Discord bot receives actual fabric output (not "queued")
- [ ] Multi-step pipeline results aggregated correctly
- [ ] Timeout handling returns partial status gracefully
- [ ] `split` branches return all results
- [ ] Nested `call` pipelines wait correctly
- [ ] Performance: Sync overhead < 50ms (just polling/aggregation)
- [ ] No breaking changes to existing async pipelines
- [ ] Documentation updated with examples

## Why This Saves the Project

**Before this RFC**: ductile is unusable for interactive use cases. Discord integration fails, web UIs can't get results, CLI tools are stuck. The async-only architecture is a dealbreaker for an entire class of users.

**After this RFC**:
- ✓ Chat bots work (Discord, Slack)
- ✓ Web UIs can display results
- ✓ CLI tools can block and print output
- ✓ Interactive workflows are first-class citizens
- ✓ Architecture remains clean (no hacky HTTP-level workarounds)
- ✓ Reuses existing DAG infrastructure (minimal new code)
- ✓ Declarative YAML config (stays true to design philosophy)

**This isn't just a feature addition - it's a fundamental capability unlock.**

## Alternatives Considered

### Alternative 1: HTTP `?wait=true` (RFC-91)
**Rejected**: Imperative, client-driven, doesn't leverage pipeline DAG, doesn't scale to complex workflows.

### Alternative 2: Webhook callbacks
**Rejected**: Requires callback infrastructure, doesn't help CLI tools, adds latency.

### Alternative 3: Separate sync service
**Rejected**: Duplicates code, operational complexity, doesn't solve root problem.

### Alternative 4: SSE/WebSocket
**Rejected**: Overkill, major protocol change, unnecessary for simple request/response.

**Why pipeline execution modes win**: Declarative, reuses existing infrastructure, aligns with design, solves complex DAG cases, minimal code.

## Next Steps

1. **Review & approve** this RFC (get buy-in from maintainer)
2. **Prototype** `WaitForCompletion` in dispatcher (1 day)
3. **Implement** Phase 1 (core infrastructure, 3-5 days)
4. **Test** with Discord bot (validate end-to-end)
5. **Document** in USER_GUIDE.md with examples
6. **Ship** as 0.2.0 milestone

## Narrative
- 2026-02-13: Created after discovering async-only architecture blocks interactive use cases. Proposes pipeline-level execution modes as elegant solution that reuses DAG infrastructure. User feedback: "maybe save the project." (by @assistant)
