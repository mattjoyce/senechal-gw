---
id: 118
status: done
priority: Medium
blocked_by: []
assignee: ""
tags: [api, jobs, diagnostics, observability]
---

# #118: Add `GET /jobs` List Endpoint

The API currently only exposes `GET /job/{jobID}` for individual job lookup. There is no way to list jobs via the API — only via the TUI (`ductile system monitor`). This is a gap for diagnostics, external tooling, and plugin-level observability.

## Motivation

Discovered when trying to inspect scheduled job state for the withings plugin deployment. To verify that `poll:withings` is running on schedule, token refresh is happening, and data fetches are occurring, you need either:
- The TUI (requires terminal access to the host)
- Direct SQLite query (`ductile.db`)

A `GET /jobs` endpoint makes this inspectable from anywhere — curl, scripts, or an LLM tool call.

## Proposed Endpoint

### `GET /jobs`

List jobs, optionally filtered.

**Query params:**
- `plugin` — filter by plugin name (e.g. `?plugin=withings`)
- `command` — filter by command (e.g. `?command=poll`)
- `status` — filter by status (`queued`, `running`, `succeeded`, `failed`, `timed_out`, `dead`) with aliases (`pending`, `ok`, `error`)
- `limit` — max results (default: 50)

**Response:**
```json
{
  "jobs": [
    {
      "job_id": "uuid",
      "plugin": "withings",
      "command": "poll",
      "status": "succeeded",
      "created_at": "ISO8601",
      "started_at": "ISO8601",
      "completed_at": "ISO8601",
      "attempt": 1
    }
  ],
  "total": 42
}
```

**Scope required:** `jobs:ro` or `*`

## Acceptance Criteria

- [x] `GET /jobs` returns paginated job list from SQLite
- [x] Supports `plugin`, `command`, `status`, `limit` query params
- [x] Sorted by `created_at` descending (most recent first)
- [x] Existing `GET /job/{jobID}` unchanged
- [x] Documented in `docs/API_REFERENCE.md`

## Narrative

- 2026-02-22: Card created. Gap discovered during withings plugin deployment planning — no API way to verify scheduled job execution without TUI or direct DB access.
- 2026-02-21: Moved to doing and implementation started for `GET /jobs` endpoint, including query filtering, ordering, tests, and API docs update. (by @assistant)
- 2026-02-21: Completed end-to-end. Added queue-backed `ListJobs` filtering with total count + created_at-desc ordering, wired authenticated `GET /jobs` route/handler (including status aliases `pending|ok|error`), added API and queue tests, and documented endpoint behavior in API reference. Full `go test ./...` passes. (by @assistant)
- 2026-02-21: Re-reviewed after merging related discovery/manifest changes; behavior remains stable and tests still pass on `main`. (by @assistant)
