# Ductile: API Reference

This document provides a comprehensive reference for the Ductile REST API.

## Base URL
Default: `http://localhost:8080`

## Authentication

All API requests (except `/healthz`, `/plugins`, `/skills`, `/openapi.json`, `/.well-known/ai-plugin.json`, and `/plugin/{name}/openapi.json`) require a Bearer token in the `Authorization` header.

```http
Authorization: Bearer <your_token>
```

Ductile uses scoped tokens configured in `api.auth.tokens`, with explicit scopes (e.g., `plugin:rw`, `jobs:ro`, `events:ro`).

---

## Endpoints

### 1. Direct Plugin Execution

Execute one plugin command directly. This **bypasses pipeline routing** and enqueues exactly one job.

**Endpoint**: `POST /plugin/{plugin}/{command}`

**Required scopes**:
- `plugin:ro` for manifest `read` commands
- `plugin:rw` (or `*`) for manifest `write` commands

**Request Body**:
```json
{
  "payload": {
    "key1": "value1",
    "key2": "value2"
  }
}
```

**Fields**:
- `payload` (Object, optional): JSON object passed to the command.
- For `handle`, the server wraps payload into an `api.trigger` event envelope before enqueue.

**Response (202 Accepted)**:
```json
{
  "job_id": "uuid-v4",
  "status": "queued",
  "plugin": "plugin_name",
  "command": "command_name"
}
```

**Example (curl)**:
```bash
curl -X POST http://localhost:8080/plugin/echo/poll \
  -H "Authorization: Bearer test_token" \
  -H "Content-Type: application/json" \
  -d '{"payload":{"message":"Hello API"}}'
```

---

### 2. Explicit Pipeline Execution

Trigger a named pipeline directly.

**Endpoint**: `POST /pipeline/{pipeline}`

**Required scopes**:
- `plugin:rw` (or `*`)

**Request Body**:
```json
{
  "payload": {
    "url": "https://example.com/article"
  }
}
```

**Query Parameters**:
- `async` (Boolean, optional): If `true`, force asynchronous response.

**Behavior**:
- Pipeline entry dispatches are resolved first.
- `execution_mode: synchronous` waits for completion unless `?async=true`.
- Synchronous mode with multiple entry dispatches returns `400`; use `?async=true` for fan-out entry pipelines.

**Response (Async default - 202 Accepted)**:
```json
{
  "job_id": "uuid-v4",
  "status": "queued",
  "plugin": "pipeline",
  "command": "pipeline_name"
}
```

**Response (Synchronous success - 200 OK)**:
```json
{
  "job_id": "uuid-v4",
  "status": "succeeded",
  "duration_ms": 1250,
  "result": { "status": "ok" },
  "tree": [
    {
      "job_id": "uuid-v4",
      "plugin": "plugin_name",
      "command": "command_name",
      "status": "succeeded",
      "result": { "status": "ok" }
    }
  ]
}
```

**Response (Timeout - 202 Accepted)**:
```json
{
  "job_id": "uuid-v4",
  "status": "running",
  "timeout_exceeded": true,
  "message": "Pipeline still running after timeout."
}
```

**Example (curl)**:
```bash
curl -X POST http://localhost:8080/pipeline/url-to-fabric \
  -H "Authorization: Bearer test_token" \
  -H "Content-Type: application/json" \
  -d '{"payload":{"url":"https://example.com"}}'
```

---

### 3. Job Status and Results

Retrieve the current status and execution results of a job.

**Endpoint**: `GET /job/{job_id}`

**Response (200 OK)**:
```json
{
  "job_id": "uuid-v4",
  "status": "completed",
  "plugin": "echo",
  "command": "poll",
  "submitted_by": "api",
  "created_at": "2026-02-13T10:00:00Z",
  "started_at": "2026-02-13T10:00:01Z",
  "completed_at": "2026-02-13T10:00:02Z",
  "result": {
    "status": "ok",
    "events": [],
    "state_updates": {},
    "logs": [
      {"level": "info", "message": "Echoed: Hello API"}
    ]
  }
}
```

**Job Statuses**:
- `queued`: Awaiting dispatch.
- `running`: Currently executing.
- `succeeded`: Finished successfully.
- `failed`: Finished with an error.
- `timed_out`: Exceeded execution deadline.
- `dead`: Failed and exhausted all retries.

---

### 5. Jobs List

List jobs with optional filtering. Requires `jobs:ro`, `jobs:rw`, or `*` scope.

**Endpoint**: `GET /jobs`

**Query Parameters**:
- `plugin` (String, optional): Exact plugin name filter.
- `command` (String, optional): Exact command name filter.
- `status` (String, optional): Job status filter. Accepted values:
  - Native: `queued`, `running`, `succeeded`, `failed`, `timed_out`, `dead`
  - Aliases: `pending` -> `queued`, `ok` -> `succeeded`, `error` -> `failed`
- `limit` (Integer, optional): Max rows returned. Default: `50`.

**Response (200 OK)**:
```json
{
  "jobs": [
    {
      "job_id": "uuid-v4",
      "plugin": "withings",
      "command": "poll",
      "status": "succeeded",
      "created_at": "2026-02-21T10:00:00Z",
      "started_at": "2026-02-21T10:00:01Z",
      "completed_at": "2026-02-21T10:00:02Z",
      "attempt": 1
    }
  ],
  "total": 42
}
```

Results are sorted by `created_at` descending (most recent first).

---

### 6. System Health

Unauthenticated endpoint for health checks. Typically used by monitoring tools or load balancers.

**Endpoint**: `GET /healthz`

**Response (200 OK)**:
```json
{
  "status": "ok",
  "uptime_seconds": 3600,
  "queue_depth": 0,
  "plugins_loaded": 5,
  "config_path": "/etc/ductile",
  "binary_path": "/usr/local/bin/ductile",
  "version": "0.1.0-dev"
}
```

---

### 7. OpenAPI Discovery

Unauthenticated endpoints for agent-driven capability discovery. Two-tier design:
- **`/plugins`** — lightweight catalog for initial discovery (semantic signaling, minimal tokens)
- **`/skills`** — unified skill index (atomic plugin skills + orchestrated pipeline skills)
- **`/openapi.json`** — global OpenAPI 3.1 spec for all plugins
- **`/plugin/{name}/openapi.json`** — scoped OpenAPI 3.1 spec for one chosen plugin
- **`/.well-known/ai-plugin.json`** — OpenAI-style discovery manifest that points at `/openapi.json`

#### Well-Known AI Plugin Manifest
**Endpoint**: `GET /.well-known/ai-plugin.json`

Returns service metadata for LLM agents and links to the global OpenAPI document.

**Response (200 OK)**:
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

#### Global OpenAPI
**Endpoint**: `GET /openapi.json`

Returns an OpenAPI 3.1 document for every discovered plugin command.

#### Single Plugin (OpenAPI)
**Endpoint**: `GET /plugin/{name}/openapi.json`

Returns an OpenAPI 3.1 document scoped to one plugin. Use after selecting a plugin from the `/plugins` list.

**Response (200 OK)**:
```json
{
  "openapi": "3.1.0",
  "info": { "title": "Ductile Gateway", "version": "1.0" },
  "paths": {
    "/plugin/echo/poll": {
      "post": {
        "operationId": "echo__poll",
        "summary": "Poll for data",
        "tags": ["echo"],
        "requestBody": {
          "required": false,
          "content": {
            "application/json": {
              "schema": { "type": "object", "properties": { "message": { "type": "string" } } }
            }
          }
        },
        "responses": {
          "202": { "description": "Job queued" },
          "400": { "description": "Bad request" },
          "403": { "description": "Insufficient scope" }
        },
        "security": [{ "BearerAuth": [] }]
      }
    }
  },
  "components": {
    "securitySchemes": { "BearerAuth": { "type": "http", "scheme": "bearer" } }
  }
}
```

**Graceful degradation:**
- No `input_schema` in manifest → `requestBody` omitted
- No `description` on command → summary defaults to `"{plugin}: {command}"`

Returns `404` if the plugin is not found.

---

### 8. Plugin Discovery

List available plugins and retrieve their metadata/schemas. The list endpoints are unauthenticated to support lightweight agent discovery.

#### List Plugins
**Endpoint**: `GET /plugins` — **No auth required**

**Response (200 OK)**:
```json
{
  "plugins": [
    {
      "name": "echo",
      "version": "0.1.0",
      "description": "A demonstration plugin",
      "commands": ["poll", "health"]
    }
  ]
}
```

#### Get Plugin Details
**Endpoint**: `GET /plugin/{name}`

Requires `plugin:ro`, `plugin:rw`, or `*` scope.

**Response (200 OK)**:
```json
{
  "name": "echo",
  "version": "0.1.0",
  "description": "A demonstration plugin",
  "protocol": 2,
  "commands": [
    {
      "name": "poll",
      "type": "write",
      "description": "Emits echo.poll events",
      "input_schema": {
        "type": "object",
        "properties": {
          "message": { "type": "string" }
        }
      }
    }
  ]
}
```

---

### 9. Skills Index

Unified, operator-facing capability index across both atomic plugin commands and named pipelines.

#### List Skills
**Endpoint**: `GET /skills` — **No auth required**

**Response (200 OK)**:
```json
{
  "skills": [
    {
      "name": "plugin.echo.poll",
      "kind": "plugin",
      "description": "Emits echo.poll events",
      "endpoint": "/plugin/echo/poll",
      "tier": "WRITE",
      "plugin": "echo",
      "command": "poll"
    },
    {
      "name": "pipeline.discord-fabric",
      "kind": "pipeline",
      "endpoint": "/pipeline/discord-fabric",
      "pipeline": "discord-fabric",
      "trigger": "discord.message",
      "execution_mode": "synchronous",
      "timeout_secs": 30
    }
  ]
}
```

Pipeline entries default to `execution_mode: "asynchronous"` when unset in config.

---

## Error Codes

- `401 Unauthorized`: Missing or invalid Bearer token.
- `403 Forbidden`: Token is valid but lacks the necessary scope for the requested action.
- `404 Not Found`: The requested plugin, command, or job ID does not exist.
- `400 Bad Request`: Invalid JSON body or missing required fields.
- `500 Internal Server Error`: An unexpected error occurred on the server.
