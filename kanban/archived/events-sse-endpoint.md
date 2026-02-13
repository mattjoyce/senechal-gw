---
id: 33
status: done
priority: High
blocked_by: []
tags: [sprint-3, api, observability, events]
---

# SSE /events Endpoint for Real-Time Observability

Implement Server-Sent Events (SSE) endpoint for streaming system events. Enables real-time monitoring, TUI development, and debugging of state machine transitions.

## Acceptance Criteria

- GET /events endpoint streams SSE events (text/event-stream)
- Authentication via same bearer token as other API endpoints
- Event broker publishes to all connected SSE clients
- Events emitted for: job state transitions, scheduler ticks, plugin lifecycle, router matches
- Ring buffer (configurable size, default 100) for late-joining clients
- Graceful client disconnect handling
- Event format: `event: <type>\ndata: <json>\n\n`
- Optional config flag `service.events.enabled` (default true)

## Use Cases

**1. Real-Time Debugging:**
```bash
# Watch state machine in real-time
curl -N -H "Authorization: Bearer $API_KEY" http://localhost:8080/events
```

**2. TUI Monitor:**
- Subscribe to /events for live updates
- No polling needed for job queue, scheduler ticks, plugin status

**3. Sprint 4 Development:**
- Observe circuit breaker state transitions
- Watch retry backoff timing
- Validate deduplication logic

## Event Types

### Job State Transitions
```
event: job.enqueued
data: {"job_id": "uuid", "plugin": "withings", "command": "poll", "enqueued_at": "..."}

event: job.started
data: {"job_id": "uuid", "started_at": "..."}

event: job.completed
data: {"job_id": "uuid", "status": "ok", "duration_ms": 234, "completed_at": "..."}

event: job.failed
data: {"job_id": "uuid", "error": "timeout", "retry": true, "retry_count": 2, "failed_at": "..."}

event: job.dead
data: {"job_id": "uuid", "reason": "max_retries_exceeded", "abandoned_at": "..."}
```

### Scheduler Events
```
event: scheduler.tick
data: {"tick_at": "...", "scheduled_count": 3}

event: scheduler.scheduled
data: {"plugin": "withings", "command": "poll", "job_id": "uuid", "jitter_ms": 2341}

event: scheduler.skipped
data: {"plugin": "garmin", "reason": "circuit_breaker_open", "failures": 5}

event: scheduler.poll_guard
data: {"plugin": "withings", "reason": "job_already_queued", "existing_job_id": "uuid"}
```

### Plugin Lifecycle
```
event: plugin.spawned
data: {"job_id": "uuid", "plugin": "withings", "command": "poll", "pid": 12345}

event: plugin.timeout
data: {"job_id": "uuid", "plugin": "withings", "timeout_sec": 30, "killed_at": "..."}

event: plugin.invalid_response
data: {"job_id": "uuid", "plugin": "withings", "error": "malformed JSON", "stderr": "..."}

event: plugin.circuit_breaker
data: {"plugin": "withings", "state": "open", "consecutive_failures": 3, "cooldown_until": "..."}
```

### Router Events (Sprint 2+)
```
event: router.match
data: {"source_plugin": "withings", "event_type": "weight_updated", "matched_routes": 2}

event: router.enqueued
data: {"target_plugin": "garmin", "command": "handle", "job_id": "uuid", "source_event": "..."}
```

### Webhook Events (Sprint 3)
```
event: webhook.received
data: {"path": "/webhook/github", "hmac_valid": true, "body_size": 1234, "remote_addr": "..."}

event: webhook.rejected
data: {"path": "/webhook/github", "reason": "hmac_invalid", "remote_addr": "..."}
```

## Implementation Details

**Package:** `internal/events`

**Event Broker:**
```go
package events

type Event struct {
    Type string      `json:"type"`
    Data interface{} `json:"data"`
    Timestamp time.Time `json:"timestamp"`
}

type Broker struct {
    clients    map[chan Event]bool
    register   chan chan Event
    unregister chan chan Event
    events     chan Event
    buffer     *RingBuffer  // Last N events for late joiners
    mu         sync.RWMutex
}

func NewBroker(bufferSize int) *Broker
func (b *Broker) Start(ctx context.Context)
func (b *Broker) Publish(typ string, data interface{})
func (b *Broker) Subscribe() <-chan Event
func (b *Broker) Unsubscribe(ch <-chan Event)
```

**Ring Buffer:**
```go
type RingBuffer struct {
    events []Event
    size   int
    head   int
    mu     sync.RWMutex
}

func (rb *RingBuffer) Push(e Event)
func (rb *RingBuffer) GetRecent() []Event  // For late-joining clients
```

**HTTP Handler (in internal/api):**
```go
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // Subscribe to broker
    eventCh := s.broker.Subscribe()
    defer s.broker.Unsubscribe(eventCh)

    // Send buffered events first
    for _, e := range s.broker.GetRecentEvents() {
        fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, marshal(e.Data))
        w.(http.Flusher).Flush()
    }

    // Stream live events
    for {
        select {
        case <-r.Context().Done():
            return
        case event := <-eventCh:
            fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, marshal(event.Data))
            w.(http.Flusher).Flush()
        }
    }
}
```

**Wiring into Existing Code:**

1. **Queue (internal/queue):**
```go
// In queue.Enqueue
broker.Publish("job.enqueued", map[string]interface{}{
    "job_id": jobID,
    "plugin": job.Plugin,
    "command": job.Command,
})

// In queue.MarkStarted
broker.Publish("job.started", map[string]interface{}{
    "job_id": jobID,
})

// In queue.MarkCompleted / MarkFailed
broker.Publish("job.completed", ...)
broker.Publish("job.failed", ...)
```

2. **Scheduler (internal/scheduler):**
```go
// In tick loop
broker.Publish("scheduler.tick", map[string]interface{}{
    "tick_at": time.Now(),
    "scheduled_count": len(scheduled),
})

// When circuit breaker prevents scheduling
broker.Publish("scheduler.skipped", map[string]interface{}{
    "plugin": name,
    "reason": "circuit_breaker_open",
})
```

3. **Dispatch (internal/dispatch):**
```go
// After spawn
broker.Publish("plugin.spawned", map[string]interface{}{
    "job_id": jobID,
    "plugin": job.Plugin,
    "pid": cmd.Process.Pid,
})

// On timeout
broker.Publish("plugin.timeout", ...)
```

**Configuration:**
```yaml
service:
  events:
    enabled: true        # Enable SSE endpoint
    buffer_size: 100     # Ring buffer for late joiners
```

**Main.go Integration:**
- Create Broker in main(), pass to API server and queue/scheduler/dispatch
- Start broker goroutine: `go broker.Start(ctx)`

## Testing

**Unit Tests:**
- Broker: publish to multiple clients, unsubscribe, ring buffer
- Handler: SSE format, auth required, graceful disconnect

**Integration Test:**
```go
// Start server, subscribe to /events
client := subscribeToEvents(t, apiKey)

// Trigger action (enqueue job)
queue.Enqueue(...)

// Assert event received
event := <-client.Events
assert.Equal("job.enqueued", event.Type)
```

**Manual Verification:**
```bash
# Terminal 1: Start service
./ductile start

# Terminal 2: Subscribe to events
curl -N -H "Authorization: Bearer $API_KEY" \
  http://localhost:8080/events

# Terminal 3: Trigger job
curl -X POST -H "Authorization: Bearer $API_KEY" \
  http://localhost:8080/trigger/echo/poll

# Observe in Terminal 2:
# event: job.enqueued
# data: {"job_id": "...", "plugin": "echo", "command": "poll"}
#
# event: job.started
# data: {"job_id": "..."}
#
# event: job.completed
# data: {"job_id": "...", "status": "ok", "duration_ms": 123}
```

## Dependencies

- Existing API server (Sprint 2 ✓)
- Auth middleware (Sprint 2 ✓)
- Queue state machine (Sprint 1 ✓)
- Scheduler (Sprint 1 ✓)
- Dispatch (Sprint 1 ✓)

## Branch

`feature/events-sse`

## Follow-On Work

This endpoint enables:
- **Simple TUI** (card #34) - Real-time monitor consuming /events
- **Enhanced diagnostics** - Tail events filtered by type/plugin
- **Future: Event filtering** - Query params like `?types=job.failed,plugin.timeout`
- **Future: Token scopes** (card #35) - Limit tokens to specific plugins/commands

## Narrative

The /events endpoint transforms debugging from "grep logs and guess" to "watch the system breathe in real-time." When implementing circuit breakers (Sprint 4), you'll immediately see `scheduler.skipped` events with failure counts. When OAuth refresh fails, you'll see `plugin.invalid_response` with the error. The 50-line broker implementation pays dividends across all future development and ops work.
