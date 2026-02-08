---
id: 20
status: todo
priority: High
blocked_by: [9, 12, 13, 16, 17]
tags: [sprint-1, mvp, testing]
---

# Echo Plugin + E2E Runbook

Create a trivial `echo` plugin and a small runbook to validate the end-to-end loop works.

## Acceptance Criteria
- `plugins/echo/manifest.yaml` and runnable entrypoint exist.
- Plugin reads request JSON from stdin and writes a valid response JSON to stdout.
- `state_updates.last_run` (or similar) persists to SQLite across runs.
- Runbook covers: normal run, plugin failure path, and hung plugin timeout test (per MVP).

## Narrative

