---
id: 122
status: done
priority: High
blocked_by: []
tags: [bug, auth, config]
---

# Bug: API Auth Tokens Not Merged from Include Files

## Symptom

Tokens defined in included config files (e.g. `api.yaml`, `tokens.yaml`) were silently
dropped. Only the `api_key` legacy field survived the include merge. All scoped tokens
returned 401 on protected endpoints (`/events`, `/jobs`, `/job/{id}`, etc.).

Public endpoints (`/healthz`, `/plugins`) were unaffected as they bypass `authMiddleware`.

## Root Cause

`deepMergeConfig` in `internal/config/loader.go` handled `API.Auth.APIKey` but had no
case for `API.Auth.Tokens`. The tokens array from every include file was silently ignored.

## Fix

Added token append in `deepMergeConfig` (`internal/config/loader.go`):

```go
if len(src.API.Auth.Tokens) > 0 {
    dst.API.Auth.Tokens = append(dst.API.Auth.Tokens, src.API.Auth.Tokens...)
}
```

## Discovered

2026-02-22 — during ops session investigating TUI connect/disconnect on `/events`.
The `Ductilian` dev token was returning 401 despite being present in `api.yaml`.

## Related

- improvement-121-config-reload (tokens must survive reload too)
