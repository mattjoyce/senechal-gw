---
id: improvement-117
status: done
priority: medium
tags: [api, openapi, discovery, agents]
---

# #117 OpenAPI Discovery Endpoints

## Goal

Agents operating the Ductile Gateway need to discover available plugins and understand how to invoke them. Add `GET /openapi.json` (all plugins) and `GET /plugin/{name}/openapi.json` (one plugin) returning standard OpenAPI 3.1 documents — unauthenticated, like `/healthz`.

## Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/openapi.json` | None | Full OpenAPI 3.1 doc — all plugins, all commands |
| `GET` | `/plugin/{name}/openapi.json` | None | OpenAPI 3.1 doc for a single plugin |

## Document Shape

Each plugin command → one `POST /plugin/{name}/{command}` path with:
- `operationId`: `{plugin}__{command}`
- `summary`: command description, or `"{plugin}: {command}"` if absent
- `requestBody`: only if `input_schema` is present (expanded via `GetFullInputSchema()`)
- `security`: `[{ "BearerAuth": [] }]`

## Files Changed

- `internal/api/openapi.go` (new) — `buildOpenAPIDoc` / `buildPluginPaths`
- `internal/api/handlers.go` — `handleOpenAPIAll`, `handleOpenAPIPlugin`
- `internal/api/server.go` — route registration (outside auth group)
- `internal/api/openapi_test.go` (new) — unit + handler tests
- `docs/API_REFERENCE.md` — document new endpoints

## Acceptance Criteria

- [x] `GET /openapi.json` returns OpenAPI 3.1 for all plugins without authentication.
- [x] `GET /plugin/{name}/openapi.json` returns OpenAPI 3.1 for a single plugin without authentication.
- [x] Generated operations include `operationId`, summary fallback, optional `requestBody`, and bearer security metadata.
- [x] Discovery routes are registered outside the auth middleware group.
- [x] Endpoint behavior is covered by API tests and documented.

## Narrative

- 2026-02-21: Reviewed implementation and closed card. Global and per-plugin OpenAPI discovery endpoints are live, unauthenticated, and covered by handler/unit tests.
- 2026-02-21: Follow-on discovery work (`/.well-known/ai-plugin.json`) and manifest compaction was completed in card #119; this card remains the foundational OpenAPI discovery slice.
