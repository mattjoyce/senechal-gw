---
id: 118
status: backlog
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
- `status` — filter by status (`pending`, `running`, `ok`, `error`)
- `limit` — max results (default: 50)

**Response:**
```json
{
  "jobs": [
    {
      "job_id": "uuid",
      "plugin": "withings",
      "command": "poll",
      "status": "ok",
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

- [ ] `GET /jobs` returns paginated job list from SQLite
- [ ] Supports `plugin`, `command`, `status`, `limit` query params
- [ ] Sorted by `created_at` descending (most recent first)
- [ ] Existing `GET /job/{jobID}` unchanged
- [ ] Documented in `docs/API_REFERENCE.md`

## Narrative

- 2026-02-22: Card created. Gap discovered during withings plugin deployment planning — no API way to verify scheduled job execution without TUI or direct DB access.
