# improvement-109: agenticloop watch should auto-detect active run

## Status
backlog

## Context
`agenticloop watch <run_id>` requires the user to supply a run ID manually.
The expected UX is: `agenticloop watch` with no arguments finds and watches any
currently active (running/queued) run automatically.

## Gap
The store already has `ListByStatus(ctx, status)` but there is no API endpoint
exposing it. The CLI watch command requires a positional `<run_id>` argument.

## Required Changes

### 1. API — add list endpoint
`GET /v1/runs?status=running` (or `?status=queued,running`)
- Wire `RunStore.ListByStatus` into a new handler in `internal/api/handlers.go`
- Register route in `setupRoutes()`: `r.Get("/v1/runs", s.handleListRuns)`
- Return JSON array of run objects

### 2. CLI — make run_id optional in watch
When no run ID supplied:
- Query `GET /v1/runs?status=running`
- If exactly one result → watch it automatically (print run ID to stderr)
- If zero → error: "no active run"
- If multiple → error: "multiple active runs, specify a run_id" (list IDs)

## Branch
`feat/agenticloop-watch-sse-tui`
