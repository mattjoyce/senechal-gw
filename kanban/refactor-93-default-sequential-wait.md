---
id: 93
status: hold
priority: High
blocked_by: []
tags: [architecture, refactor, pipelines, breaking-change, github-actions, on-hold]
---

**STATUS: ON HOLD** - Critique raised concerns about breaking event-driven architecture and HTTP blocking issues. See RFC-92 for alternative approach.

**Critique Summary**:
- Conflicts with event-driven model (engine/runner.go)
- HTTP blocking causes resource starvation
- Misapplies GitHub Actions pattern (GHA workflow trigger is async)
- Too risky for ecosystem breakage

**Next Steps**: Flesh out RFC-92 with explicit execution modes (async default, sync opt-in).

---

# Refactor: Default Sequential Steps to Wait (GitHub Actions Pattern)

## Problem

Current behavior is **unintuitive and breaks interactive use cases**:
- Sequential pipeline steps fire-and-forget (async by default)
- Parent jobs return immediately, children run in background
- Impossible to get results from triggered workflows
- Contradicts how every other workflow engine works (GitHub Actions, Tekton, Argo, Airflow)

See RFC-91 and RFC-92 for full problem analysis.

## Proposed Change

**Adopt GitHub Actions pattern**: Sequential steps wait for completion by default.

### Current Behavior (Async Default)

```yaml
steps:
  - uses: fabric
  - uses: formatter
```

**Today**: Both queued immediately, parent returns, no waiting.

### New Behavior (Sync Default)

```yaml
steps:
  - uses: fabric      # Runs first
  - uses: formatter   # Waits for fabric to complete, then runs
```

**After refactor**: Each step waits for previous step to complete before starting next.

## Design: Industry Standard Alignment

Following **GitHub Actions / Tekton / Argo** conventions:

### Sequential (Default - Wait)

```yaml
steps:
  - uses: step1  # Runs first
  - uses: step2  # Waits for step1
  - uses: step3  # Waits for step2
```

### Parallel (Explicit Keyword)

```yaml
steps:
  - parallel:  # NEW keyword (or keep 'split:')
      - uses: branch-a
      - uses: branch-b
      - uses: branch-c
    # All three run in parallel, next step waits for ALL to complete

  - uses: next-step  # Waits for a, b, c
```

### Async Fire-and-Forget (Explicit Opt-In)

```yaml
steps:
  - uses: background-task
    async: true  # NEW: Explicit fire-and-forget

  - uses: next-step  # Runs immediately, doesn't wait for background-task
```

## Trigger Endpoint Behavior

### Design Decision: Hybrid Approach

**Sync by default** (unlike GitHub Actions), **async optional** (for GitHub-style workflows).

**Rationale**: senechal-gw is a personal automation tool, not enterprise CI/CD. Most use cases want immediate results (Discord bots, CLI tools, web UIs), not polling loops.

### Current (Broken)

```bash
POST /trigger/fabric/handle
→ HTTP 202 {"job_id": "...", "status": "queued"}
# Job runs async, caller never gets result ❌
```

### After Refactor: Synchronous by Default

```bash
POST /trigger/fabric/handle
→ HTTP 200 {"job_id": "...", "status": "succeeded", "result": {...}}
# Waits for pipeline to complete, returns actual results ✓
```

**Timeout handling**: If pipeline exceeds timeout (default 30s), return 202 with partial status:

```bash
POST /trigger/fabric/handle
→ HTTP 202 {"job_id": "...", "status": "running", "timeout_exceeded": true}
# Timeout: Pipeline still running, poll /job/{id} for status
```

### Optional: Async Mode (GitHub Actions Style)

```bash
POST /trigger/fabric/handle?async=true
→ HTTP 202 {"job_id": "...", "status": "queued"}
# Returns immediately, caller must poll /job/{id}
```

**Use async mode for**:
- Long-running pipelines (> 2 minutes)
- CI/CD workflows triggered by git push
- Background batch processing
- When you don't need immediate results

### Query Params

| Param | Behavior | Use Case |
|-------|----------|----------|
| (none) | **Sync** - wait for completion (default) | Discord bots, CLI, web UIs |
| `?async=true` | Async - return immediately | Long-running workflows, CI/CD |
| `?timeout=60s` | Custom timeout (overrides default 30s) | Longer pipelines |

## Baggage / Context Accumulation

**No change** - results still flow to expanding baggage in database:

- ✓ `event_context` table accumulates metadata across steps
- ✓ Immutable `origin_*` fields preserved
- ✓ Workspace artifacts still cloned between steps
- ✓ Parent-child lineage still tracked

**What changes**: When parent job is marked "complete" and returns to caller.

**Before**: Parent completes immediately after spawning children
**After**: Parent completes after all descendant jobs complete

## Implementation Plan

### Phase 1: Dispatcher Refactor (Core Logic)

**File**: `internal/dispatch/dispatcher.go`

```go
// Current: Parent job spawns children and returns immediately
func (d *Dispatcher) executeStep(job *Job, step Step) error {
    childJobID := d.queueJob(step.Plugin, step.Command, job.ID)
    // Parent continues immediately ❌
    return nil
}

// New: Parent job waits for children based on step type
func (d *Dispatcher) executeStep(job *Job, step Step) error {
    if step.Async {
        // Explicit fire-and-forget
        childJobID := d.queueJob(step.Plugin, step.Command, job.ID)
        return nil
    }

    if step.Parallel != nil {
        // Parallel execution
        childIDs := d.queueParallelJobs(step.Parallel, job.ID)
        return d.waitForAll(childIDs)  // NEW: Wait for all branches ✓
    }

    // Default: Sequential (wait)
    childJobID := d.queueJob(step.Plugin, step.Command, job.ID)
    return d.waitForJob(childJobID)  // NEW: Wait for completion ✓
}

func (d *Dispatcher) waitForJob(jobID string) error {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            job := d.getJob(jobID)
            if job.Status == "succeeded" || job.Status == "failed" {
                return job.Error
            }
        }
    }
}
```

### Phase 2: Config Schema Update

**File**: `internal/config/types.go`

```go
type Step struct {
    ID      string
    Uses    string  // Plugin name
    Call    string  // Nested pipeline
    Parallel []Step  // NEW: Renamed from 'Split'
    Async   bool    // NEW: Explicit fire-and-forget flag
    OnEvents map[string][]string
}
```

### Phase 3: Pipeline Parsing

**File**: `internal/config/loader.go`

Support both old and new syntax for backward compatibility:

```go
func parseStep(node yaml.Node) (Step, error) {
    // Support 'parallel:' (new) and 'split:' (old, deprecated)
    if node.HasField("parallel") || node.HasField("split") {
        return Step{Parallel: parseSteps(node)}, nil
    }

    // Support 'async: true' flag
    async := node.GetBool("async", false)

    return Step{Uses: ..., Async: async}, nil
}
```

### Phase 4: API Handler Updates

**File**: `cmd/senechal-gw/api/handlers.go`

```go
func (h *Handler) Trigger(w http.ResponseWriter, r *http.Request) {
    jobID := h.dispatcher.QueueJob(...)

    // NEW: Check for async mode via query param
    asyncMode := r.URL.Query().Get("async") == "true"

    if asyncMode {
        // GitHub Actions style: Return immediately
        respondJSON(w, 202, AsyncResponse{
            JobID:  jobID,
            Status: "queued",
            Plugin: pluginName,
            Command: commandName,
        })
        return
    }

    // Default: Wait for pipeline completion (synchronous)
    timeout := 30 * time.Second  // Default timeout

    // Allow custom timeout via query param
    if timeoutParam := r.URL.Query().Get("timeout"); timeoutParam != "" {
        if d, err := time.ParseDuration(timeoutParam); err == nil {
            timeout = d
        }
    }

    result, err := h.dispatcher.WaitForJob(jobID, timeout)

    if err == ErrTimeout {
        // Timeout exceeded: Return partial status
        respondJSON(w, 202, TimeoutResponse{
            JobID:           jobID,
            Status:          "running",
            TimeoutExceeded: true,
            ElapsedMs:       int(timeout.Milliseconds()),
            Message:         "Pipeline still running. Check /job/{id} for status.",
        })
        return
    }

    if err != nil {
        // Job failed
        respondJSON(w, 500, ErrorResponse{
            JobID:  jobID,
            Status: "failed",
            Error:  err.Error(),
        })
        return
    }

    // Success: Return complete result
    respondJSON(w, 200, SuccessResponse{
        JobID:      jobID,
        Status:     result.Status,
        DurationMs: result.Duration.Milliseconds(),
        Result:     result,
        Children:   result.Children,
    })
}
```

### Phase 5: Testing

**Test cases**:
1. **Default sync mode**: Sequential steps wait in order, API returns 200 with results
2. **Parallel steps**: Run concurrently, parent waits for all branches
3. **Step-level async**: `async: true` on step skips waiting for that step
4. **Timeout handling**: Pipeline exceeds timeout, returns 202 with partial status
5. **Async query param**: `?async=true` returns 202 immediately (GitHub-style)
6. **Custom timeout**: `?timeout=60s` overrides default 30s timeout
7. **Nested pipelines**: `call:` steps wait correctly for child pipeline
8. **Discord integration**: End-to-end test with actual bot receiving results
9. **Baggage accumulation**: Verify context still flows through event_context table
10. **Backward compatibility**: Existing pipelines with `split:` still work

## Breaking Changes

⚠️ **This is a breaking change** for existing pipelines:

**Before**: Sequential steps fire-and-forget
**After**: Sequential steps wait for completion

**Migration path**:
1. Bump version to 0.2.0 (breaking change)
2. Add migration guide: "If you want old behavior, add `async: true` to steps"
3. Most users will want new behavior (it's more intuitive)

## Backward Compatibility (Optional)

Could add global config flag to preserve old behavior:

```yaml
# config.yaml
pipeline_defaults:
  sequential_mode: wait  # 'wait' (new default) or 'async' (old behavior)
```

But **recommend**: Just make the breaking change. Current behavior is broken anyway.

## Success Criteria

- [ ] Sequential steps wait for previous step completion
- [ ] Parallel steps (`parallel:` keyword) run concurrently
- [ ] Async flag (`async: true`) enables fire-and-forget
- [ ] Trigger endpoints return results (HTTP 200) for completed pipelines
- [ ] Trigger endpoints return partial status (HTTP 202) on timeout
- [ ] Discord bot receives actual fabric output (not "queued")
- [ ] Baggage still accumulates in `event_context` table
- [ ] Parent-child lineage still tracked
- [ ] Test suite passes

## Discord Use Case (Solved)

```yaml
# pipelines/discord-fabric.yaml
pipelines:
  - name: discord-fabric
    on: fabric.handle
    steps:
      - uses: fabric  # Waits for completion (new default)
```

```python
# Discord bot - NO CHANGES NEEDED
response = requests.post("/trigger/fabric/handle", json={...})
result = response.json()

# Before: {"status": "queued"}
# After:  {"status": "succeeded", "result": {"stdout": "..."}}

await message.channel.send(result["result"]["stdout"])  # ✓ Works!
```

## Timeline

- **Week 1**: Dispatcher refactor (wait logic)
- **Week 2**: Config parsing, API handlers
- **Week 3**: Testing, migration guide
- **Release**: 0.2.0 (breaking change milestone)

## Related RFCs

- **RFC-91**: Problem statement (async-only blocks interactive use cases)
- **RFC-92**: Initial proposal (pipeline execution modes) - superseded by this refactor
- This refactor is **simpler** than RFC-92 because it changes the default rather than adding new modes

## Narrative
- 2026-02-13: Created to adopt GitHub Actions pattern (sequential = wait by default). Solves interactive use case problem (Discord, web UIs) by making intuitive behavior the default. User feedback: "ok, Adopt GitHub Actions Pattern." (by @assistant)
- 2026-02-13: Technical review complete. Findings: 1) Dispatcher requires `WaitForJob` and `waitForAll` methods to support blocking execution. 2) API handlers in `internal/api` must be updated to support the `?async=true` opt-out and blocking response. 3) Config schema needs extension for `execution_mode` and `parallel` steps (aliasing `split`). 4) Result aggregation logic is needed to collect artifacts and logs from the entire DAG subtree for the final response. Ready for Phase 1 implementation. (by @assistant)
