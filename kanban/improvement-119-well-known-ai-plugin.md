---
id: 119
status: done
priority: Medium
blocked_by: []
assignee: ""
tags: [api, discovery, agents, openai, well-known, openapi, manifests]
---

# #119: Agent Discovery — `/.well-known/ai-plugin.json` + `GET /openapi.json`

## Goal

Two complementary unauthenticated discovery endpoints for LLM agents:

1. `GET /.well-known/ai-plugin.json` — OpenAI-style manifest pointing to the OpenAPI doc
2. `GET /openapi.json` — Full OpenAPI 3.1 doc, all plugins, generated from manifests

Both unauthenticated. `GET /plugins` and `GET /plugin/{name}/openapi.json` remain alongside them.

## Background

The `/.well-known/` path is an IETF standard (RFC 8615) for service discovery. The OpenAI plugin manifest format was designed for LLM agent discovery; it is the closest established convention to ductile's use case. The global `/openapi.json` is the standard expected location for a machine-readable API spec and is referenced by the manifest's `api.url` field.

`buildOpenAPIDoc()` in `internal/api/openapi.go` already generates correct OpenAPI 3.1 from a plugin map — it just needs a global handler and route wired up.

## Part 1 — `GET /.well-known/ai-plugin.json`

### Response shape

```json
{
  "schema_version": "v1",
  "name_for_human": "Ductile Gateway",
  "name_for_model": "ductile",
  "description_for_human": "Integration gateway for triggering plugins and pipelines.",
  "description_for_model": "Discover and invoke plugins. Fetch /openapi.json for the full spec, or /plugin/{name}/openapi.json for a single plugin. Invoke commands via POST /plugin/{name}/{command}.",
  "auth": {
    "type": "bearer"
  },
  "api": {
    "type": "openapi",
    "url": "/openapi.json"
  }
}
```

### Implementation

- New handler `handleWellKnownPlugin` in `handlers.go` — returns static/derived JSON
- Route: `r.Get("/.well-known/ai-plugin.json", s.handleWellKnownPlugin)` outside auth group

## Part 2 — `GET /openapi.json` (global)

### Implementation

- New handler `handleOpenAPIAll` in `handlers.go`:
  ```go
  func (s *Server) handleOpenAPIAll(w http.ResponseWriter, r *http.Request) {
      doc := buildOpenAPIDoc(s.registry.All())
      respondJSON(w, http.StatusOK, doc)
  }
  ```
- Route: `r.Get("/openapi.json", s.handleOpenAPIAll)` outside auth group

## Part 3 — Manifest Compaction

### Problem

Manifests are inconsistent. Some use a full command object form (echo), others use a bare string list (fabric, file_handler, youtube_transcript, jina-reader partially). The bare list loses `description` and `input_schema`, producing low-quality OpenAPI output (no summaries, no request bodies).

### Current formats

**Full (echo):**
```yaml
commands:
  - name: poll
    type: write
    description: "Emits echo.poll events and updates the internal last_run timestamp."
    input_schema:
      message: string
```

**Bare list (fabric, file_handler, youtube_transcript):**
```yaml
commands: [poll, handle, health]
```

### Target: compact object form

All manifests should use the object form, but stripped to only what's needed for discovery and OpenAPI generation. Fields like `output_schema` are optional.

```yaml
commands:
  - name: poll
    description: "One-line description for agents."
    input_schema:           # omit if no meaningful input
      key: type
  - name: handle
    description: "One-line description for agents."
    input_schema:
      key: type
  - name: health
    description: "Returns plugin health and version."
```

### Plugins to update

- [x] `plugins/fabric/manifest.yaml` — added descriptions + input_schema
- [x] `plugins/file_handler/manifest.yaml` — added descriptions + input_schema
- [x] `plugins/youtube_transcript/manifest.yaml` — added descriptions + input_schema
- [x] `plugins/jina-reader/manifest.yaml` — added descriptions + input_schema for `handle`
- [x] `plugins/echo/manifest.yaml` — retained full object form (including optional `output_schema`)

## Acceptance Criteria

- [x] `GET /.well-known/ai-plugin.json` returns valid manifest (no auth)
- [x] `GET /openapi.json` returns full OpenAPI 3.1 doc for all plugins (no auth)
- [x] All plugin manifests use compact object form with `name` + `description` minimum
- [x] OpenAPI output has meaningful `summary` and `requestBody` for all commands that have input
- [x] Both routes registered outside auth group in `server.go`
- [x] Documented in `docs/API_REFERENCE.md`

## Narrative

- 2026-02-22: Card created. Identified during review of agent discovery conventions.
- 2026-02-22: Expanded to include global `/openapi.json` (handler missing from #117 implementation) and manifest compaction work — bare-list manifests produce low-quality OpenAPI output.
- 2026-02-21: Moved to doing and queued behind #118 implementation so job listing API can land first, then discovery/manifest compaction follow-up. (by @assistant)
- 2026-02-21: Completed discovery endpoints by adding unauthenticated `GET /openapi.json` and `GET /.well-known/ai-plugin.json`, with route registration outside auth and focused API tests for both handlers. (by @assistant)
- 2026-02-21: Merged latest `main` manifest uplift commit (`25d2225`) during implementation to align compaction work with upstream changes for `fabric`, `file_handler`, `youtube_transcript`, and `jina-reader`, then revalidated full test suite. (by @assistant)
- 2026-02-21: Follow-on hardening added explicit manifest schema metadata (`manifest_spec`, `manifest_version`) across all first-party manifests to keep discovery inputs versioned and consistent as plugin contracts evolve. (by @assistant)
