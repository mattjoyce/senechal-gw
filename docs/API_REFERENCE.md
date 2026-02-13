# Senechal Gateway: API Reference

This document provides a comprehensive reference for the Senechal Gateway REST API.

## Base URL
Default: `http://localhost:8080`

## Authentication

All API requests (except `/healthz`) require a Bearer token in the `Authorization` header.

```http
Authorization: Bearer <your_token>
```

Senechal supports two authentication modes:
1. **Legacy API Key**: A single `api_key` configured in `api.auth`.
2. **Scoped Tokens**: A list of `tokens` with specific scopes (e.g., `read:*`, `plugin:rw`).

---

## Endpoints

### 1. Manual Plugin Execution

Trigger a plugin command immediately. This enqueues a job in the work queue.

**Endpoint**: `POST /trigger/{plugin}/{command}`

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
- `payload` (Object, required): The JSON object that will be passed to the plugin. If the command is `handle`, this payload is automatically wrapped in an `api.trigger` event envelope.

**Query Parameters**:
- `async` (Boolean, optional): If `true`, forces the request to return immediately even if a synchronous pipeline is matched.

**Response (Default / Async - 202 Accepted)**:
```json
{
  "job_id": "uuid-v4",
  "status": "queued",
  "plugin": "plugin_name",
  "command": "command_name"
}
```

**Response (Synchronous Pipeline - 200 OK)**:
If the trigger matches a pipeline configured with `execution_mode: synchronous`, the API will block and return the full execution tree.
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
    },
    {
      "job_id": "child-uuid",
      "plugin": "notifier",
      "command": "handle",
      "status": "succeeded",
      "result": { "status": "ok" }
    }
  ]
}
```

**Response (Timeout - 202 Accepted)**:
If a synchronous pipeline exceeds its `timeout` (or the system `max_sync_timeout`), it returns partial status.
```json
{
  "job_id": "uuid-v4",
  "status": "running",
  "timeout_exceeded": true,
  "message": "Pipeline still running after timeout. Check /job/uuid-v4"
}
```

**Example (curl)**:
```bash
curl -X POST http://localhost:8080/trigger/echo/poll 
  -H "Authorization: Bearer test_token" 
  -H "Content-Type: application/json" 
  -d '{"payload": {"message": "Hello API"}}'
```

---

### 2. Job Status and Results

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

### 3. System Health

Unauthenticated endpoint for health checks. Typically used by monitoring tools or load balancers.

**Endpoint**: `GET /healthz`

**Response (200 OK)**:
```json
{
  "status": "ok",
  "uptime_seconds": 3600,
  "queue_depth": 0,
  "plugins_loaded": 5
}
```

---

## Error Codes

- `401 Unauthorized`: Missing or invalid Bearer token.
- `403 Forbidden`: Token is valid but lacks the necessary scope for the requested action.
- `404 Not Found`: The requested plugin, command, or job ID does not exist.
- `400 Bad Request`: Invalid JSON body or missing required fields.
- `500 Internal Server Error`: An unexpected error occurred on the server.
