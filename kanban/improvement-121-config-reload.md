---
id: 121
status: todo
priority: Normal
blocked_by: []
tags: [improvement, ops, api]
---

# Config Reload Without Restart

## Problem

Config changes (tokens, plugin settings, pipelines) currently require `docker restart ductile`, which:
- Drops all active SSE connections (TUI disconnects)
- Interrupts in-flight jobs
- Creates operational friction when rotating tokens or tuning plugins

Discovered during 2026-02-22 ops session: the `tokens.yaml` include was missing from `config.yaml`, causing 401 on `/events`. Fix required a restart, disconnecting the TUI.

## Proposed Solution

Add a `SIGHUP`-triggered config reload (or a `POST /admin/reload` API endpoint) that:
1. Re-reads all config files (including `tokens.yaml`, `plugins.yaml`, `pipelines.yaml`)
2. Updates in-memory state (token list, plugin registry, pipeline routes)
3. Does **not** restart the HTTP server or drop SSE connections

## Acceptance Criteria

- [ ] `docker kill --signal=SIGHUP ductile` reloads config without restarting
- [ ] Active SSE clients remain connected through a reload
- [ ] New tokens are effective immediately after reload
- [ ] Invalid config on reload logs an error and retains previous config (safe fallback)
- [ ] Optionally: `POST /admin/reload` endpoint for API-triggered reload

## Notes

- Scope: token list, plugin config, pipeline definitions — not DB path or listen address (those require restart)
- Must be safe to call while jobs are running
