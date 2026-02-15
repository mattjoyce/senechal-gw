# Bug 101 — API trigger handler does not create EventContext for pipeline jobs

**Status:** backlog
**Priority:** high
**Created:** 2026-02-15

## Description

When a pipeline is triggered via the API endpoint `POST /trigger/{plugin}/{command}`,
no EventContext is created for the initial job. This means multi-step pipelines
only execute step 1 — subsequent steps are never dispatched.

## Root Cause

`internal/api/handlers.go` — `handleTrigger()` enqueues a job without setting
`EventContextID`:

```go
jobID, err := s.queue.Enqueue(r.Context(), queue.EnqueueRequest{
    Plugin:      pluginName,
    Command:     commandName,
    Payload:     enqueuePayload,
    SubmittedBy: "api",
    // EventContextID is NOT set
})
```

The `EnqueueRequest` struct already has an `EventContextID` field — it's just
never populated by the API handler.

## Chain of Failure

1. API trigger enqueues job with `EventContextID = nil`
2. Step 1 runs, emits events
3. `dispatcher.routeEvents()` (line ~476) checks `job.EventContextID` — nil,
   so `sourcePipeline` and `sourceStepID` remain `""`
4. `router.Next()` (engine.go ~96-113) skips intra-pipeline successor logic
   because `SourcePipeline == ""`
5. Step 2 is never discovered or dispatched

## Why E2E Tests Pass

E2E tests trigger pipelines through the normal event-driven path (plugin emits
event → router matches trigger → creates EventContext for step 1), not through
the API trigger endpoint.

## Fix

In `handleTrigger()`, before enqueuing:

1. Check if the trigger matches a pipeline via `router.GetPipelineByTrigger()`
2. If yes, create an initial EventContext via `ContextStore.Create()`
3. Pass the `EventContextID` in the `EnqueueRequest`

## Reproduction

```bash
# Pipeline: url-to-fabric (jina-reader → fabric), triggered by jina-reader.handle
curl -X POST http://localhost:8080/trigger/jina-reader/handle \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com", "pattern": "summarize"}'

# Result: jina-reader runs (step 1), fabric never runs (step 2)
# Evidence: event_context table is empty, all jobs have event_context_id = NULL
```

## Files Involved

- `internal/api/handlers.go` — needs to create EventContext
- `internal/dispatch/dispatcher.go:469-581` — routeEvents() relies on EventContextID
- `internal/router/engine.go:96-113` — Next() skips successors when SourcePipeline empty
- `internal/state/context_store.go` — ContextStore.Create() method
- `internal/queue/types.go` — EnqueueRequest already has EventContextID field
