---
id: 28
status: done
priority: High
blocked_by: []
assignee: "@claude"
tags: [sprint-2, api, http]
---

# HTTP Server + API Endpoints

Implement HTTP server with API endpoints for LLM/external triggers. Enables curl-based job triggering and result retrieval.

## Acceptance Criteria

- HTTP server starts alongside scheduler/dispatcher in main.go
- Graceful shutdown on SIGINT/SIGTERM (cancels context)
- POST /trigger/{plugin}/{command} endpoint enqueues job, returns job_id
- GET /job/{job_id} endpoint returns job status and results
- Authentication middleware validates API key from header
- Error handling for invalid plugin/command, missing auth
- Integration with existing queue and plugin registry
- Comprehensive tests (handler tests, E2E)

## Implementation Details

**Package:** `internal/api`

**HTTP Server:**
- Use `github.com/go-chi/chi/v5` router (lightweight, idiomatic)
- Graceful shutdown via context cancellation
- Start in main.go after scheduler/dispatcher
- Listen address from config: `api.listen` (default: localhost:8080)

**POST /trigger/{plugin}/{command}:**
```go
Request:
- URL params: plugin name, command name
- Body: JSON payload (optional)
- Header: Authorization: Bearer <api_key>

Response (202 Accepted):
{
  "job_id": "uuid",
  "status": "queued",
  "plugin": "plugin_name",
  "command": "command_name"
}

Response (400 Bad Request):
{
  "error": "plugin not found"
}

Response (401 Unauthorized):
{
  "error": "invalid API key"
}
```

**GET /job/{job_id}:**
```go
Response (200 OK - running):
{
  "job_id": "uuid",
  "status": "running",
  "plugin": "plugin_name",
  "command": "command_name",
  "started_at": "2026-02-09T10:00:00Z"
}

Response (200 OK - completed):
{
  "job_id": "uuid",
  "status": "completed",
  "plugin": "plugin_name",
  "command": "command_name",
  "result": {...},  // Plugin response payload
  "started_at": "2026-02-09T10:00:00Z",
  "completed_at": "2026-02-09T10:00:05Z"
}

Response (404 Not Found):
{
  "error": "job not found"
}
```

**Dependencies:**
- Queue interface (already exists): `Enqueue()`, `GetJobByID()` (Agent 2 adds this)
- Plugin registry (already exists): `Get(name)`
- Auth function (Agent 2 provides): `ValidateAPIKey(key string) bool`

**Configuration:**
```yaml
api:
  enabled: true
  listen: "localhost:8080"
  auth:
    api_key: ${API_KEY}  # From environment
```

**Main.go Integration:**
- Start API server in goroutine after scheduler/dispatcher
- Pass context for graceful shutdown
- Wire to same queue and plugin registry

**Testing:**
- Unit tests for each handler (mock queue/registry)
- Auth middleware tests
- Integration test: start server → POST /trigger → verify job enqueued
- E2E test: trigger echo plugin → poll for results

## Interface Contract (with Agent 2)

Agent 2 (Codex) provides:

```go
// Job storage enhancement
type JobResult struct {
    JobID       string
    Status      queue.Status
    Plugin      string
    Command     string
    Result      json.RawMessage  // Plugin response payload
    StartedAt   time.Time
    CompletedAt *time.Time
}

func (q *Queue) GetJobByID(ctx context.Context, jobID string) (*JobResult, error)

// Auth function
func ValidateAPIKey(key string, configKey string) bool
```

Agent 1 (Claude) uses these interfaces to implement API handlers.

## Branch

`claude/api-server`

## Verification

```bash
# Start server
./ductile start --config config.yaml

# Trigger echo plugin
curl -X POST http://localhost:8080/trigger/echo/poll \
  -H "Authorization: Bearer test-key-123" \
  -H "Content-Type: application/json" \
  -d '{}'

# Returns: {"job_id": "abc-123", "status": "queued", ...}

# Get results
curl http://localhost:8080/job/abc-123 \
  -H "Authorization: Bearer test-key-123"

# Returns: {"job_id": "abc-123", "status": "completed", "result": {...}}
```

## Narrative
