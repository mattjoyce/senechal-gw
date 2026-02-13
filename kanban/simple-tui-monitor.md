---
id: 34
status: backlog
priority: Normal
blocked_by: [33]
tags: [sprint-3, tui, monitoring, observability]
---

# Simple TUI Monitor

Build a terminal UI for real-time monitoring of Ductile. Provides live visibility into job queue, plugin states, scheduler activity, and recent events without grepping logs.

## Acceptance Criteria

- Live dashboard showing:
  - Service health (uptime, queue depth, plugins loaded)
  - Active/recent jobs table (status, plugin, duration)
  - Scheduler next tick countdown
  - Circuit breaker states (if Sprint 4 implemented)
  - Event stream tail (last 20 events, scrollable)
- Keyboard navigation:
  - `↑/↓` - Navigate job list
  - `Enter` - View job details (logs, payload, result)
  - `p` - View plugin states
  - `e` - Focus event stream
  - `q` - Quit
- Auto-refresh via /events SSE stream (no polling)
- Graceful degradation if /events unavailable (poll /healthz every 5s)
- Optional: Color-coded status (green=ok, red=failed, yellow=running)

## Layout Mockup

```
┌─ Ductile ────────────────────────────────────────────────┐
│ Status: Running  │  Uptime: 2h 34m  │  Queue: 2 pending  │  Load: 0.3 │
└───────────────────────────────────────────────────────────────────┘

┌─ Active Jobs ─────────────────────────────────────────────────────┐
│ ID       │ Plugin    │ Command │ Status    │ Duration  │ Started   │
├──────────┼───────────┼─────────┼───────────┼───────────┼───────────┤
│ abc-123  │ withings  │ poll    │ ✓ ok      │ 234ms     │ 10:23:45  │
│ def-456  │ garmin    │ handle  │ ⏳ running│ 1.2s      │ 10:24:01  │
│ ghi-789  │ withings  │ poll    │ ✗ failed  │ 5.0s      │ 10:20:12  │
└───────────────────────────────────────────────────────────────────┘

┌─ Scheduler ───────────────────────────────────────────────────────┐
│ Next tick: 00:42 │ Circuit breakers: garmin (open, 5 failures)    │
└───────────────────────────────────────────────────────────────────┘

┌─ Event Stream ────────────────────────────────────────────────────┐
│ 10:24:03  job.started       │ job_id=def-456 plugin=garmin        │
│ 10:24:01  scheduler.tick    │ scheduled=2 plugins                 │
│ 10:23:45  job.completed     │ job_id=abc-123 duration=234ms       │
│ 10:23:30  plugin.timeout    │ job_id=xyz-999 killed after 30s     │
│ 10:23:12  webhook.received  │ path=/webhook/github hmac=valid     │
└───────────────────────────────────────────────────────────────────┘

[↑/↓] Navigate  [Enter] Details  [p] Plugins  [e] Events  [q] Quit
```

## Implementation Details

**Library:** [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- Elm-architecture TUI framework (idiomatic Go)
- Composable views, message-driven updates
- Well-maintained, used by Charm.sh tools

**Package:** `internal/tui`

**Main Loop:**
```go
type Model struct {
    events     <-chan events.Event  // SSE subscription
    jobs       []JobSummary         // Recent jobs
    health     HealthStatus         // From /healthz or /events
    eventLog   []events.Event       // Ring buffer
    selectedJob int                 // Cursor position
    view       ViewMode             // jobs | plugins | events
}

func (m Model) Init() tea.Cmd {
    return tea.Batch(
        subscribeToEvents,
        tick(5 * time.Second),  // Fallback polling
    )
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case events.Event:
        return m.handleEvent(msg)
    case tea.KeyMsg:
        return m.handleKeyPress(msg)
    }
}

func (m Model) View() string {
    return lipgloss.JoinVertical(
        lipgloss.Left,
        m.renderHeader(),
        m.renderJobTable(),
        m.renderScheduler(),
        m.renderEventStream(),
        m.renderFooter(),
    )
}
```

**Event Handling:**
```go
func (m *Model) handleEvent(e events.Event) (Model, tea.Cmd) {
    // Update state based on event type
    switch e.Type {
    case "job.enqueued":
        m.addJob(e.Data)
    case "job.completed", "job.failed":
        m.updateJob(e.Data)
    case "scheduler.tick":
        m.updateScheduler(e.Data)
    }

    // Add to event log
    m.eventLog = append([]events.Event{e}, m.eventLog[:min(19, len(m.eventLog))]...)
    return *m, nil
}
```

**SSE Subscription:**
```go
func subscribeToEvents(apiURL, apiKey string) tea.Cmd {
    return func() tea.Msg {
        client := &http.Client{}
        req, _ := http.NewRequest("GET", apiURL+"/events", nil)
        req.Header.Set("Authorization", "Bearer "+apiKey)

        resp, err := client.Do(req)
        if err != nil {
            return errMsg{err}
        }

        // Parse SSE stream, send events as tea.Msg
        scanner := bufio.NewScanner(resp.Body)
        for scanner.Scan() {
            if event := parseSSE(scanner.Text()); event != nil {
                // Send event to Bubble Tea runtime
            }
        }
    }
}
```

**Fallback Polling (if /events unavailable):**
```go
func pollHealthz(apiURL, apiKey string) tea.Cmd {
    return func() tea.Msg {
        // GET /healthz every 5s
        health := fetchHealthz(apiURL, apiKey)
        return healthMsg{health}
    }
}
```

**Configuration:**
```yaml
# In user's shell config or .env
export DUCTILE_API_URL="http://localhost:8080"
export DUCTILE_API_KEY="your-api-key"
```

**CLI Integration:**
```bash
# New subcommand
./ductile monitor [--api-url URL] [--api-key KEY]
```

## Dependencies

- /events SSE endpoint (card #33) - **BLOCKS**
- /healthz endpoint (Sprint 3) - For fallback polling
- Existing API endpoints (Sprint 2 ✓) - GET /job/{id}

**Go Dependencies:**
```go
require (
    github.com/charmbracelet/bubbletea v0.25.0
    github.com/charmbracelet/lipgloss v0.9.1
)
```

## Testing

**Manual:**
```bash
# Start service
./ductile start

# In another terminal, start TUI
./ductile monitor

# Trigger jobs, watch them appear in TUI
curl -X POST http://localhost:8080/trigger/echo/poll
```

**Unit Tests:**
- Event handling logic (mock events, assert state updates)
- View rendering (snapshot tests for layouts)

## Deferred Features

**Not in v1:**
- Filtering events by type/plugin (add later if needed)
- Historical job search (use CLI or API directly)
- Interactive job retry/cancel (dangerous, keep read-only)
- Customizable layout/themes (YAGNI)

## Use Cases

**Development:**
- Watch scheduler ticks while tuning jitter/intervals
- Observe circuit breaker open/close during plugin failures
- Debug OAuth refresh flows (see plugin.invalid_response + state updates)

**Operations:**
- Quick health check without SSH + log grep
- Monitor queue depth during deployment
- Verify webhooks are being received (see webhook.received events)

## Narrative

The TUI transforms "how's my gateway doing?" from a multi-step investigation (SSH, grep logs, check DB) into a single `./ductile monitor` command. It's especially valuable during Sprint 4 (reliability controls) development—watching circuit breakers trip and recover in real-time makes tuning thresholds intuitive. Since it's just a consumer of the /events endpoint, it's isolated from core logic and can be enhanced over time without risk.

**Scope:** This is a nice-to-have enhancement, not a blocker for production. Build after /events is proven stable (Sprint 3 complete). If time-constrained, defer to post-Sprint 4.
