---
id: 92
status: backlog
priority: High
blocked_by: []
tags: [architecture, pipelines, interactive, dag, critical, implementation-ready]
---

# RFC-92: Pipeline Execution Modes (Explicit Opt-In)

## Status Update (2026-02-13)

**Refactor-93 on hold** due to critique concerns. This RFC proposes alternative approach:
- **Async by default** (preserves current event-driven architecture)
- **Sync opt-in** (explicit per-pipeline)
- **No breaking changes** (existing pipelines unchanged)

## Executive Summary

**Problem**: senechal-gw's async-only execution blocks interactive use cases (Discord bots, web UIs, CLI tools). Users get "queued" responses but never receive actual results.

**Solution**: Add optional `execution_mode: synchronous` to pipeline config. Pipelines explicitly declare whether they should wait for completion before returning results.

**Key Principle**: **Async by default** (preserve event-driven architecture), **sync by opt-in** (enable interactive use cases).

## Design Philosophy

### Preserve Event-Driven Architecture

**Current architecture (keep this)**:
- Events trigger pipelines
- Steps emit events (`on_events`)
- Dispatcher queues jobs asynchronously
- Parent jobs don't block on children

**What we add**:
- Optional synchronous wait at API boundary (trigger endpoints)
- Internal event flow remains unchanged
- Dispatcher still processes jobs asynchronously
- Only difference: HTTP handler waits for completion before responding

### When to Use Each Mode

| Mode | Use Cases | Example Pipelines |
|------|-----------|-------------------|
| **async** (default) | Batch processing, webhooks, long-running jobs, fire-and-forget | File uploads, scheduled jobs, background processing |
| **synchronous** (opt-in) | Interactive commands, chat bots, CLI tools, web APIs | Discord commands, CLI queries, API endpoints that return results |

## Proposed Schema

### Pipeline Configuration

```yaml
pipelines:
  # Async pipeline (default, no change to existing configs)
  - name: batch-processor
    on: file.upload
    steps:
      - uses: processor
      - uses: archiver

  # Synchronous pipeline (explicit opt-in)
  - name: discord-fabric
    on: fabric.handle
    execution_mode: synchronous  # NEW: Explicit sync mode
    timeout: 30s                 # NEW: Max wait time
    steps:
      - uses: fabric
      - uses: formatter
```

### Configuration Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `execution_mode` | `async` \| `synchronous` | `async` | How trigger endpoint behaves |
| `timeout` | duration | `30s` | Max wait time for synchronous pipelines |

## How It Works

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Trigger Endpoint                         │
│  POST /trigger/fabric/handle                                │
└────────────┬────────────────────────────────────────────────┘
             │
             ├──> Lookup pipeline config
             │    - execution_mode: synchronous?
             │    - timeout: 30s
             │
             ├──> Queue job (existing dispatcher)
             │    - Job ID: abc-123
             │    - Status: queued
             │
             ├──> If async: return 202 immediately ──────────┐
             │                                                │
             └──> If sync: wait for completion               │
                  │                                           │
                  ├─> Poll job status (100ms intervals)      │
                  ├─> Check timeout                          │
                  └─> Return results when complete           │
                                                              │
┌─────────────────────────────────────────────────────────────┘
│                Event-Driven Execution (Unchanged)           │
│  ┌──────────┐      ┌──────────┐      ┌──────────┐         │
│  │  Job 1   │─────>│  Job 2   │─────>│  Job 3   │         │
│  │ (fabric) │event │(formatter)│event │  (save)  │         │
│  └──────────┘      └──────────┘      └──────────┘         │
│                                                             │
│  - Jobs queued asynchronously                              │
│  - Events trigger next steps                               │
│  - Dispatcher processes queue                              │
│  - Parent-child relationships tracked                      │
└─────────────────────────────────────────────────────────────┘
```

### Key Insight: Wait at API Boundary, Not Internal Execution

**Internal execution remains event-driven**:
- Dispatcher still queues jobs asynchronously
- Events still trigger next steps
- No blocking in dispatcher/runner

**Wait happens only at HTTP layer**:
- API handler polls job status
- Returns results when complete
- Or returns timeout if exceeded

This preserves event-driven architecture while enabling sync API responses.

## Implementation Plan

### Phase 1: Config Schema (Week 1, Day 1-2)

**Files to modify**:

#### `internal/config/types.go`

```go
type Pipeline struct {
    Name          string
    On            string
    Steps         []Step
    ExecutionMode ExecutionMode  // NEW
    Timeout       time.Duration  // NEW
}

type ExecutionMode string

const (
    ExecutionModeAsync ExecutionMode = "async"
    ExecutionModeSync  ExecutionMode = "synchronous"
)
```

#### `internal/config/loader.go`

```go
func loadPipeline(node yaml.Node) (*Pipeline, error) {
    p := &Pipeline{
        ExecutionMode: ExecutionModeAsync, // Default to async
        Timeout:       30 * time.Second,   // Default timeout
    }

    // Parse execution_mode if present
    if modeStr := node.GetString("execution_mode"); modeStr != "" {
        switch modeStr {
        case "async":
            p.ExecutionMode = ExecutionModeAsync
        case "synchronous":
            p.ExecutionMode = ExecutionModeSync
        default:
            return nil, fmt.Errorf("invalid execution_mode: %s", modeStr)
        }
    }

    // Parse timeout if present
    if timeoutStr := node.GetString("timeout"); timeoutStr != "" {
        timeout, err := time.ParseDuration(timeoutStr)
        if err != nil {
            return nil, fmt.Errorf("invalid timeout: %w", err)
        }
        p.Timeout = timeout
    }

    return p, nil
}
```

### Phase 2: Job Completion Tracking (Week 1, Day 3-4)

**Files to modify**:

#### `internal/dispatch/dispatcher.go`

Add ability to wait for job tree completion:

```go
type Dispatcher struct {
    db          *sql.DB
    queue       chan *Job
    completions map[string]chan JobResult  // NEW: Completion notifications
    mu          sync.RWMutex
}

// WaitForJobTree waits for root job and all descendant jobs to complete
func (d *Dispatcher) WaitForJobTree(ctx context.Context, rootJobID string, timeout time.Duration) (*JobResult, error) {
    // Create completion channel for this job
    completionChan := make(chan JobResult, 1)

    d.mu.Lock()
    d.completions[rootJobID] = completionChan
    d.mu.Unlock()

    defer func() {
        d.mu.Lock()
        delete(d.completions, rootJobID)
        d.mu.Unlock()
    }()

    // Set up timeout
    timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    // Wait for completion or timeout
    select {
    case result := <-completionChan:
        return &result, nil
    case <-timeoutCtx.Done():
        return nil, ErrTimeout
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

// notifyCompletion called when job finishes (existing job completion logic)
func (d *Dispatcher) notifyCompletion(jobID string, result JobResult) {
    d.mu.RLock()
    if ch, exists := d.completions[jobID]; exists {
        select {
        case ch <- result:
        default:
        }
    }
    d.mu.RUnlock()
}

// checkJobTreeComplete checks if root job + all children are done
func (d *Dispatcher) checkJobTreeComplete(rootJobID string) bool {
    // Query database for job tree
    rows, err := d.db.Query(`
        WITH RECURSIVE job_tree AS (
            SELECT id, status FROM job_queue WHERE id = ?
            UNION ALL
            SELECT jq.id, jq.status
            FROM job_queue jq
            JOIN job_tree jt ON jq.parent_job_id = jt.id
        )
        SELECT COUNT(*) as total,
               SUM(CASE WHEN status IN ('succeeded', 'failed') THEN 1 ELSE 0 END) as complete
        FROM job_tree
    `, rootJobID)

    if err != nil {
        return false
    }
    defer rows.Close()

    var total, complete int
    if rows.Next() {
        rows.Scan(&total, &complete)
    }

    return total > 0 && total == complete
}
```

**Key Design Decision**:
- Don't poll in tight loop (wasteful)
- Use completion notifications (event-driven)
- When a job completes, check if it's being waited on
- Notify waiting HTTP handler via channel

### Phase 3: API Handler Updates (Week 1, Day 5)

**Files to modify**:

#### `cmd/senechal-gw/api/handlers.go`

```go
func (h *Handler) TriggerPlugin(w http.ResponseWriter, r *http.Request) {
    // Parse request, queue job (existing logic)
    jobID := h.dispatcher.QueueJob(pluginName, commandName, payload)

    // Lookup pipeline to check execution mode
    pipeline := h.router.GetPipelineForTrigger(pluginName, commandName)

    // Default to async if no pipeline config
    if pipeline == nil || pipeline.ExecutionMode == config.ExecutionModeAsync {
        // Async mode: return immediately (current behavior)
        respondJSON(w, http.StatusAccepted, AsyncResponse{
            JobID:   jobID,
            Status:  "queued",
            Plugin:  pluginName,
            Command: commandName,
        })
        return
    }

    // Synchronous mode: wait for completion
    timeout := pipeline.Timeout
    if timeout == 0 {
        timeout = 30 * time.Second // Fallback default
    }

    result, err := h.dispatcher.WaitForJobTree(r.Context(), jobID, timeout)

    if err == dispatch.ErrTimeout {
        // Timeout: Pipeline still running
        respondJSON(w, http.StatusAccepted, TimeoutResponse{
            JobID:           jobID,
            Status:          "running",
            TimeoutExceeded: true,
            TimeoutSeconds:  int(timeout.Seconds()),
            Message:         "Pipeline still running after timeout. Check /job/" + jobID,
        })
        return
    }

    if err != nil {
        // Other error
        respondJSON(w, http.StatusInternalServerError, ErrorResponse{
            Error: err.Error(),
        })
        return
    }

    // Success: Return complete results
    respondJSON(w, http.StatusOK, SyncResponse{
        JobID:      jobID,
        Status:     result.Status,
        DurationMs: result.Duration.Milliseconds(),
        Result: JobResultData{
            Stdout:      result.Stdout,
            Stderr:      result.Stderr,
            ExitCode:    result.ExitCode,
            Context:     result.Context,
            OutputFiles: result.OutputFiles,
        },
        Children: result.Children,
    })
}
```

### Phase 4: Response Types (Week 1, Day 5)

**Files to modify**:

#### `cmd/senechal-gw/api/types.go`

```go
// AsyncResponse for async pipelines (existing, unchanged)
type AsyncResponse struct {
    JobID   string `json:"job_id"`
    Status  string `json:"status"`
    Plugin  string `json:"plugin"`
    Command string `json:"command"`
}

// SyncResponse for synchronous pipelines (NEW)
type SyncResponse struct {
    JobID      string        `json:"job_id"`
    Status     string        `json:"status"`
    DurationMs int64         `json:"duration_ms"`
    Result     JobResultData `json:"result"`
    Children   []ChildJob    `json:"children,omitempty"`
}

type JobResultData struct {
    Stdout      string            `json:"stdout"`
    Stderr      string            `json:"stderr,omitempty"`
    ExitCode    int               `json:"exit_code"`
    Context     map[string]any    `json:"context"`
    OutputFiles []OutputFileInfo  `json:"output_files,omitempty"`
}

type OutputFileInfo struct {
    Path string `json:"path"`
    Size int64  `json:"size"`
}

type ChildJob struct {
    JobID   string `json:"job_id"`
    Plugin  string `json:"plugin"`
    Status  string `json:"status"`
}

// TimeoutResponse for sync pipelines that exceed timeout (NEW)
type TimeoutResponse struct {
    JobID           string `json:"job_id"`
    Status          string `json:"status"`
    TimeoutExceeded bool   `json:"timeout_exceeded"`
    TimeoutSeconds  int    `json:"timeout_seconds"`
    Message         string `json:"message"`
}
```

### Phase 5: Resource Management (Week 2)

**Concern from critique**: HTTP blocking causes resource starvation.

**Mitigation strategies**:

#### 1. Limit Concurrent Synchronous Requests

```go
// cmd/senechal-gw/api/server.go
type Server struct {
    syncSemaphore chan struct{} // Limit concurrent sync requests
}

func NewServer(maxConcurrentSync int) *Server {
    return &Server{
        syncSemaphore: make(chan struct{}, maxConcurrentSync),
    }
}

func (s *Server) TriggerPlugin(w http.ResponseWriter, r *http.Request) {
    // ... determine if sync mode ...

    if pipeline.ExecutionMode == config.ExecutionModeSync {
        // Acquire semaphore
        select {
        case s.syncSemaphore <- struct{}{}:
            defer func() { <-s.syncSemaphore }()
        default:
            // Too many concurrent sync requests
            respondJSON(w, http.StatusServiceUnavailable, ErrorResponse{
                Error: "Too many concurrent synchronous requests. Try again or use async mode.",
            })
            return
        }

        // ... proceed with sync wait ...
    }
}
```

#### 2. Enforce Maximum Timeout

```yaml
# config.yaml
api:
  max_sync_timeout: 2m      # Never wait longer than 2 minutes
  max_concurrent_sync: 10   # Max 10 blocking requests at once
```

#### 3. Separate HTTP Server Pools (Future)

```go
// Different listener for sync vs async (advanced)
http.ListenAndServe(":8080", asyncHandler)  // Async endpoints
http.ListenAndServe(":8081", syncHandler)   // Sync endpoints
```

### Phase 6: Testing (Week 2)

**Test cases**:

1. **Async pipeline (default)**:
   - POST /trigger → Returns 202 immediately
   - Job runs in background
   - No waiting

2. **Sync pipeline (simple)**:
   - Single-step pipeline with `execution_mode: synchronous`
   - POST /trigger → Returns 200 with result
   - Duration < timeout

3. **Sync pipeline (multi-step)**:
   - Sequential steps: download → process → save
   - POST /trigger → Returns 200 with aggregated result
   - Includes children in response

4. **Sync pipeline (timeout)**:
   - Long-running pipeline
   - POST /trigger → Returns 202 after 30s timeout
   - Job continues running in background

5. **Sync pipeline (parallel branches)**:
   - Pipeline with `split:` keyword
   - POST /trigger → Waits for all branches
   - Returns aggregated results

6. **Resource limits**:
   - 11 concurrent sync requests (limit is 10)
   - 11th request gets 503 Service Unavailable

7. **Discord integration (end-to-end)**:
   - Discord bot sends /ai command
   - Gateway processes with sync mode
   - Bot receives actual fabric output

## What Needs to Change (File Checklist)

### Core Implementation

- [ ] `internal/config/types.go` - Add ExecutionMode, Timeout fields
- [ ] `internal/config/loader.go` - Parse execution_mode, timeout
- [ ] `internal/dispatch/dispatcher.go` - Add WaitForJobTree, completion notifications
- [ ] `cmd/senechal-gw/api/handlers.go` - Check execution mode, wait for sync pipelines
- [ ] `cmd/senechal-gw/api/types.go` - Add SyncResponse, TimeoutResponse types
- [ ] `cmd/senechal-gw/api/server.go` - Add semaphore for rate limiting

### Configuration

- [ ] `config.yaml` - Add api.max_sync_timeout, api.max_concurrent_sync

### Documentation

- [ ] `docs/PIPELINES.md` - Document execution_mode field
- [ ] `docs/USER_GUIDE.md` - Examples of sync vs async pipelines
- [ ] `docs/API.md` - Document new response formats

### Testing

- [ ] `internal/dispatch/dispatcher_test.go` - Test WaitForJobTree
- [ ] `cmd/senechal-gw/api/handlers_test.go` - Test sync/async modes
- [ ] `test/integration/` - End-to-end Discord bot test

### Migration

- [ ] No migration needed (async is default, existing configs unchanged)

## Addressing the Critique

### ✅ Preserves Event-Driven Architecture

- Internal dispatcher/runner unchanged
- Events still trigger steps
- Jobs still queued asynchronously
- Only wait at API boundary, not internal execution

### ✅ Prevents HTTP Resource Starvation

- Semaphore limits concurrent sync requests
- Enforced max timeout (2 minutes)
- Can separate sync/async onto different ports (future)

### ✅ Follows GitHub Actions Pattern Correctly

- Workflow trigger can be async (default) or sync (opt-in)
- Internal step execution uses existing event flow
- No conflation of trigger mechanism and step semantics

### ✅ No Ecosystem Breakage

- Async is default (existing pipelines unchanged)
- Sync is explicit opt-in (new feature)
- No breaking changes

### ✅ Reduces Complexity

- Clear semantics: pipeline declares its execution mode
- No ambiguity in step behavior
- Explicit timeouts prevent hung connections

## Migration Path

### Existing Pipelines (No Changes)

```yaml
# All existing pipelines default to async
pipelines:
  - name: existing-pipeline
    on: some.event
    steps:
      - uses: plugin1
      - uses: plugin2
# Behavior: Returns 202 immediately (unchanged)
```

### New Interactive Pipelines (Opt-In)

```yaml
# Explicitly declare sync mode
pipelines:
  - name: discord-fabric
    on: fabric.handle
    execution_mode: synchronous  # NEW
    timeout: 30s                 # NEW
    steps:
      - uses: fabric
# Behavior: Returns 200 with results (new capability)
```

## Success Criteria

- [ ] Discord bot receives actual fabric output (not "queued")
- [ ] Async pipelines unchanged (backward compatible)
- [ ] Sync pipelines return results within timeout
- [ ] HTTP resource limits enforced (no starvation)
- [ ] Event-driven architecture preserved
- [ ] Less than 100ms overhead for sync polling
- [ ] Documentation complete with examples

## Timeline

- **Week 1**: Core implementation (config, dispatcher, API handler)
- **Week 2**: Resource management, testing
- **Week 3**: Documentation, Discord integration testing
- **Release**: 0.2.0 (new feature, no breaking changes)

## Open Questions

1. **Should we support per-request override?**
   - `POST /trigger?force_async=true` to override sync pipeline
   - `POST /trigger?wait=true` to override async pipeline

2. **How to handle very long pipelines?**
   - Return partial results at timeout?
   - Provide streaming status updates?

3. **Should sync mode work with webhooks?**
   - Sync mode + callback_url = redundant?
   - Or both for belt-and-suspenders?

## Narrative

- 2026-02-13: Created to solve interactive use case problem (RFC-91)
- 2026-02-13: Refactor-93 put on hold due to critique (breaking changes, HTTP blocking)
- 2026-02-13: RFC-92 updated with detailed implementation, async default, explicit opt-in approach. Addresses all critique concerns while enabling interactive use cases.
