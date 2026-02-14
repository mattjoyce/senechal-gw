---
id: 98
status: todo
priority: High
blocked_by: []
tags: [tui, monitoring, diagnostics, mvp]
---

# Overwatch TUI - MVP (Core + Pipelines + Events)

Implement `ductile system watch` command - a real-time diagnostic TUI for monitoring gateway health, active pipelines, and event stream.

**Design:** See `/docs/TUI_WATCH_DESIGN.md` for complete specification.

## MVP Scope (Phase 1 + 2)

Ship a usable monitoring tool with:
- ✅ Live header with system health indicators
- ✅ Pipelines panel showing active jobs
- ✅ Event stream tail (last 10 events)
- ✅ Basic keyboard navigation

**Out of scope for MVP:**
- Scheduler panel (see card #99)
- Job inspector modal (see card #100)
- Advanced polish/progress bars (see card #100)

## Acceptance Criteria

### Phase 1: Core Structure ✅
- [ ] Create `internal/tui/watch/` directory structure
- [ ] Implement theme system scaffold (single default theme)
- [ ] Basic BubbleTea model with ticker (updates every 1s)
- [ ] Spinner that rotates on event arrival (●●●○○ pattern)
- [ ] Connect to SSE `/events` endpoint
- [ ] Poll `/healthz` every 5s for system status
- [ ] Header panel displays:
  - [ ] Rotating ticker `⟲` (shows system alive)
  - [ ] Health status (HEALTHY/DEGRADED/DOWN)
  - [ ] Uptime, queue depth, plugin count
  - [ ] SSE client count (requires `/healthz` field addition)
  - [ ] Last event timestamp + event spinner
  - [ ] API requests per minute

### Phase 2: Pipelines Panel ✅
- [ ] Parse config to discover pipelines
- [ ] Track active jobs from SSE events
- [ ] Display compact pipeline list:
  - [ ] Pipeline name
  - [ ] Status ([N active] or [idle])
  - [ ] Last run timestamp
  - [ ] Status icon (◉ running, ✅ succeeded, ❌ failed)
- [ ] Show running jobs with:
  - [ ] Job ID (truncated to 8 chars)
  - [ ] Current plugin/step
  - [ ] Duration counter
  - [ ] Timeout awareness (X / Y seconds)
- [ ] Keyboard navigation (↑/↓ to select pipeline)
- [ ] Expandable view (→ shows job details)

### Phase 2.5: Event Stream ✅
- [ ] Display last 10 events in reverse chronological order
- [ ] Format: `HH:MM:SS event.type Brief description [job_id] status`
- [ ] Color-coded by event type
- [ ] Auto-scroll (follow mode by default)
- [ ] Handles common events:
  - [ ] job.started, job.completed, job.failed
  - [ ] scheduler.tick
  - [ ] api.request (if available)

### Testing
- [ ] Manual testing with running gateway
- [ ] Test with multiple active pipelines
- [ ] Test SSE reconnection on disconnect
- [ ] Verify ticker/spinner show system is alive
- [ ] Test on terminals of different sizes

### Documentation
- [ ] Add `system watch` to CLI help text
- [ ] Update user guide with TUI screenshots
- [ ] Document keyboard shortcuts in help overlay

## Technical Notes

### Theme System
Define all colors in `theme.go` for future theme support:
```go
type Theme struct {
    StatusOK, StatusRunning, StatusFailed lipgloss.Style
    Border, Title, Header, Dim, Highlight lipgloss.Style
    TickerActive, TickerInactive, Progress lipgloss.Style
}
```

### SSE Client Count
Requires adding `sse_clients` field to `/healthz` response. If not available, show "N/A" for MVP.

### File Structure
```
internal/tui/watch/
├── watch.go      # Command entry point
├── model.go      # BubbleTea model
├── theme.go      # Theme definitions
├── header.go     # Header panel component
├── pipelines.go  # Pipelines panel component
├── events.go     # Event stream component
├── indicators.go # Ticker, spinner helpers
└── client.go     # API/SSE client
```

## Follow-up Cards

After MVP ships:
- **#99** - Add scheduler panel for cron job visibility
- **#100** - Add job inspector modal and polish (progress bars, etc.)

## Narrative
- 2026-02-15: Created from TUI_WATCH_DESIGN.md specification. MVP focuses on core monitoring with clean architecture for future expansion. (by @claude)
