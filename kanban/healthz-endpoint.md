---
id: 43
status: done
priority: Normal
blocked_by: []
tags: [sprint-3, observability, api]
---

# /healthz Endpoint

Implement health check endpoint for monitoring and operational visibility.

## Acceptance Criteria

- GET /healthz returns JSON status
- No authentication required (localhost binding recommended)
- Fields include:
  - uptime (duration since start)
  - queue_depth (pending jobs)
  - plugins_loaded (count)
  - circuit_breakers (plugins with open breakers)
- Returns 200 OK when healthy
- Returns 503 Service Unavailable if degraded (optional)
- Fast response time (<10ms)

## Implementation Details

**Package:** `internal/api/`

**Response Format:**
```json
{
  "status": "ok",
  "uptime_seconds": 3600,
  "queue": {
    "depth": 5,
    "oldest_job_age_seconds": 120
  },
  "plugins": {
    "loaded": 3,
    "circuit_breakers_open": ["withings"]
  },
  "timestamp": "2026-02-10T12:00:00Z"
}
```

**Health Criteria (optional):**
- OK: Queue depth < 100, no circuit breakers open
- Degraded: Queue depth > 100 OR any circuit breaker open
- Return 503 if degraded (allows load balancer health checks)

**Security:**
- No authentication (trust network boundaries)
- Recommended: Bind API to localhost only in production
- No sensitive information in response

## Testing

- GET /healthz → 200 OK with valid JSON
- Response includes all required fields
- Uptime increases over time
- Queue depth reflects actual queue state
- Circuit breaker state accurate

## Dependencies

- Existing HTTP server from Sprint 2
- Queue metrics (already available)
- Plugin registry (already available)
- Circuit breaker state (if implemented)

## Narrative

