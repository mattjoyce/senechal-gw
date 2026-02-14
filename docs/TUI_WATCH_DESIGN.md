# TUI Watch Command Design Specification

**Command:** `ductile system watch` (possibly "overwatch")
**Purpose:** Real-time diagnostic monitoring for ductile gateway
**Design Philosophy:** Low-volume, diagnostic-first, live indicators, operator-friendly

## Design Principles

1. **Show the system is alive** - ticker, spinner, live indicators
2. **Diagnostic-first** - designed for troubleshooting, not hyperscale monitoring
3. **Pipeline-centric** - visualize event chains, retries, progress
4. **Scheduler visibility** - surface cron-like scheduled jobs
5. **Connection awareness** - show SSE clients, API activity
6. **Operator UX** - inspired by systemctl, k9s, lazydocker, htop

## Architecture

### Theme System (Scaffold)

Even with a single default theme, centralize all styling for maintainability:

```go
type Theme struct {
    // Status colors
    StatusOK       lipgloss.Style
    StatusRunning  lipgloss.Style
    StatusFailed   lipgloss.Style
    StatusQueued   lipgloss.Style
    StatusDead     lipgloss.Style

    // UI elements
    Border         lipgloss.Style
    Title          lipgloss.Style
    Header         lipgloss.Style
    Dim            lipgloss.Style
    Highlight      lipgloss.Style

    // Indicators
    TickerActive   lipgloss.Style
    TickerInactive lipgloss.Style
    Progress       lipgloss.Style
}

func newDefaultTheme() Theme {
    return Theme{
        StatusOK:       lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")),
        StatusRunning:  lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")),
        StatusFailed:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")),
        StatusQueued:   lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
        StatusDead:     lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")),
        Border:         lipgloss.NewStyle().BorderForeground(lipgloss.Color("#874BFD")),
        Title:          lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA")),
        Header:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#61AFEF")),
        Dim:            lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
        Highlight:      lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B")),
        TickerActive:   lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")),
        TickerInactive: lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")),
        Progress:       lipgloss.NewStyle().Foreground(lipgloss.Color("#61AFEF")),
    }
}
```

**Benefits:**
- All colors in one place (easy to adjust palette)
- Self-documenting (see all styling decisions at a glance)
- Zero-cost abstraction for future theme support
- More readable code: `theme.StatusOK.Render("✓")` vs magic colors

### Main View Layout

```
┌─ DUCTILE WATCH ⟲ ──────────────────────────────── 15:23:45 ────┐
│ ✅ HEALTHY  ⏱ 2h 34m  Queue: 0  Plugins: 6  📡 2 clients       │
│ Last event: 2s ago ●●●○○  API: 3 req/min                       │
└─────────────────────────────────────────────────────────────────┘

┌─ PIPELINES ────────────────────────────────────────────────────┐
│ 1. discord-fabric              [1 active]  ◉ 45.2s / 60s  75% │
│    fabric.handle → fabric                                      │
│    └─ Job 09fbb6e6: Calling LLM...                            │
│ 2. file-to-report              [idle]     Last: 12m ago ✅     │
│ 3. nightly-backup              [idle]     Next: 8h 22m ⏰      │
└────────────────────────────────────────────────────────────────┘

┌─ SCHEDULED (cron) ─────────────────────────────────────────────┐
│ ⏰ Next: 2m - echo/poll (every 5m)                             │
│    Last tick: 6s ago ✓ 6 jobs scheduled                       │
│ ┌─────────────────────────────────────────────────────────────┐│
│ │ echo/poll      Every 5m    Last: 3m ago ✓   Next: 2m       ││
│ │ fabric/poll    Every 1h    Last: 12m ago ✓  Next: 48m      ││
│ │ backup/run     0 2 * * *   Last: 13h ago ✓  Next: 11h      ││
│ └─────────────────────────────────────────────────────────────┘│
└────────────────────────────────────────────────────────────────┘

┌─ EVENT STREAM ─────────────────────────────────────────────────┐
│ 15:23:43 job.started         discord-fabric [09fbb6e6]        │
│ 15:23:40 scheduler.tick      ✓ 6 jobs checked                 │
│ 15:23:12 job.completed       file-to-report [8a4c2d11] ✅ 2.1s│
│ 15:22:58 api.request         POST /pipelines/discord-fabric   │
└────────────────────────────────────────────────────────────────┘

[q] Quit • [↑/↓] Navigate • [→] Expand • [s] Toggle scheduler • [j] Jobs
```

## Component Details

### 1. Header Panel

**Purpose:** System status at a glance + live indicators

**Elements:**
- **Ticker** `⟲`: Rotates every second to show system is alive
- **Status**: Health from `/healthz` (HEALTHY/DEGRADED/DOWN)
- **Uptime**: From health endpoint
- **Queue depth**: Real-time from health
- **Plugin count**: Loaded plugins
- **Connections** `📡 N clients`: SSE subscribers (shows engagement)
- **Last event**: Shows time since last event + spinner `●●●○○` (rotates on events)
- **API activity**: Requests per minute

**Data sources:**
- `/healthz` - polled every 5s
- `/events` - SSE stream (drives spinner)
- Internal counter for API requests

### 2. Pipelines Panel

**Purpose:** Show active and recent pipeline executions

**Display modes:**

**Compact** (default):
```
1. discord-fabric    [1 active]  ◉ 45.2s / 60s  75%
2. file-to-report    [idle]     Last: 12m ago ✅
```

**Expanded** (selected pipeline):
```
1. discord-fabric              [1 active]  ◉ 45.2s / 60s  75%
   fabric.handle → fabric
   └─ Job 09fbb6e6: Calling LLM...
      Started: 45s ago
      Timeout: 60s
      Plugin: fabric
```

**Features:**
- Show active jobs with progress indicators
- Timeout awareness (show X / Y seconds, progress bar)
- Retry indicators (attempt 2/3)
- Status symbols: ◉ running, ✅ succeeded, ❌ failed, ⏳ queued
- Expand on selection to show job details
- Click job ID to jump to job inspector

### 3. Scheduled Jobs Panel

**Purpose:** Surface cron-like scheduled events (often forgotten!)

**Collapsed view:**
```
⏰ Next: 2m - echo/poll (every 5m)
   Last tick: 6s ago ✓ 6 jobs scheduled
```

**Expanded view** (toggle with 's'):
```
┌─────────────────────────────────────────────────────────────┐
│ echo/poll      Every 5m    Last: 3m ago ✓   Next: 2m       │
│ fabric/poll    Every 1h    Last: 12m ago ✓  Next: 48m      │
│ backup/run     0 2 * * *   Last: 13h ago ✓  Next: 11h      │
│ cleanup/run    0 */6 * * * Last: 2h ago ✅  Next: 4h       │
│ health/check   Every 30s   Last: 6s ago ✓   Next: 24s      │
│ report/daily   0 9 * * 1-5 Last: 6h ago ✅  Next: 18h      │
└─────────────────────────────────────────────────────────────┘
```

**Features:**
- Show next scheduled job prominently
- Last tick timestamp (shows scheduler is alive)
- Per-job schedule, last run, next run
- Status indicators: ✓ (ran), ✅ (succeeded), ❌ (failed), ⏭ (skipped)

### 4. Event Stream Panel

**Purpose:** Live tail of recent events (like `journalctl -f`)

**Format:**
```
HH:MM:SS event.type           Brief description [job_id] status
```

**Examples:**
```
15:23:43 job.started         discord-fabric [09fbb6e6]
15:23:40 scheduler.tick      ✓ 6 jobs checked
15:23:12 job.completed       file-to-report [8a4c2d11] ✅ 2.1s
15:22:58 api.request         POST /pipelines/discord-fabric
15:22:45 job.failed          backup-run [7f3e1a90] ❌ timeout
```

**Features:**
- Auto-scroll (follow mode)
- Color-coded by event type
- Click job ID to jump to inspector
- Shows last 10-20 events

### 5. Job Inspector (Modal)

**Trigger:** Press 'j' or click job ID

**Purpose:** Deep dive into a single job execution

```
┌─ JOB INSPECTOR: 09fbb6e6 ─────────────────────────────────────┐
│ Pipeline: discord-fabric                                      │
│ Status: ◉ RUNNING (45.2s / 60s timeout)                      │
│ Started: 2024-02-15 15:23:43                                  │
│                                                               │
│ ┌─ TIMELINE ─────────────────────────────────────────────────┐│
│ │ 15:23:43  job.enqueued      → Queue depth: 1             ││
│ │ 15:23:43  job.started       → fabric.handle              ││
│ │ 15:23:44  fabric.completed  → fabric plugin              ││
│ │ 15:23:45  [current]         → Waiting for LLM...         ││
│ └───────────────────────────────────────────────────────────┘│
│                                                               │
│ ┌─ PAYLOAD ──────────────────────────────────────────────────┐│
│ │ {                                                         ││
│ │   "prompt": "What does ductile mean?",                   ││
│ │   "pattern": "",                                         ││
│ │   "model": "gpt-4"                                       ││
│ │ }                                                         ││
│ └───────────────────────────────────────────────────────────┘│
│                                                               │
│ ┌─ LOGS ─────────────────────────────────────────────────────┐│
│ │ [INFO] fabric: Executing prompt-only mode               ││
│ │ [INFO] fabric: Calling OpenAI API...                    ││
│ └───────────────────────────────────────────────────────────┘│
│                                                               │
│ [ESC] Close • [↑/↓] Scroll                                   │
└───────────────────────────────────────────────────────────────┘
```

## Navigation & Keybindings

| Key | Action |
|-----|--------|
| `q`, `Ctrl+C` | Quit |
| `↑`/`↓` | Navigate pipelines list |
| `→` | Expand selected pipeline |
| `←` | Collapse pipeline |
| `s` | Toggle scheduler panel (collapsed/expanded) |
| `j` | Open job inspector for selected job |
| `ESC` | Close modal / deselect |
| `f` | Toggle event stream follow mode |

## Live Indicators

### Ticker `⟲`
- Rotates through states every 1s: `⟲ → ⟳ → ⟲`
- Shows system is responsive
- Stops rotating if no updates (indicates freeze)

### Event Spinner `●●●○○`
- Rotates on each event received
- Pattern: `●●●○○ → ○●●●○ → ○○●●● → ●○○●● → ●●○○●`
- Visual feedback that events are flowing

### Progress Bars
For running jobs with known timeouts:
```
◉ 45.2s / 60s  ████████████░░░░░░░  75%
```

## Data Sources

### SSE Event Stream (`/events`)
- Primary data source for live updates
- Drives event spinner
- Populates event stream panel
- Updates job states in real-time

### Health Endpoint (`/healthz`)
- Polled every 5 seconds
- Provides: status, uptime, queue_depth, plugins_loaded
- Plus custom field: `sse_clients` (connection count)

### Jobs API (`/jobs/:id`)
- On-demand for job inspector
- Fetch full job details, timeline, logs

### Scheduler API (`/scheduler/jobs` - if available)
- List scheduled jobs
- Show next run times
- Last execution status

## Implementation Phases

### Phase 1: Core Structure
- BubbleTea model scaffold
- Theme system
- Ticker + spinner implementation
- SSE connection
- Health polling
- Basic header panel

**Deliverable:** Header panel with live indicators working

### Phase 2: Pipelines Panel
- Parse config to list pipelines
- Track active jobs from events
- Show compact view
- Basic navigation (up/down)

**Deliverable:** Pipelines panel showing active/idle state

### Phase 3: Scheduler Panel
- Fetch scheduled jobs (API TBD)
- Show next scheduled job
- Expandable view with full schedule

**Deliverable:** Scheduler panel with toggle

### Phase 4: Event Stream
- Tail last N events
- Format event display
- Auto-scroll follow mode

**Deliverable:** Event stream panel with live updates

### Phase 5: Job Inspector
- Modal overlay
- Fetch job details
- Timeline view
- Payload + logs display

**Deliverable:** Press 'j' to inspect any job

### Phase 6: Polish
- Progress bars for timeouts
- Retry indicators
- Clickable job IDs
- Keyboard shortcuts help overlay
- Error handling and reconnection

**Deliverable:** Production-ready `ductile system watch`

## File Structure

```
internal/tui/watch/
├── watch.go          # Main command entry point
├── model.go          # BubbleTea model
├── theme.go          # Theme definitions
├── header.go         # Header panel component
├── pipelines.go      # Pipelines panel component
├── scheduler.go      # Scheduler panel component
├── events.go         # Event stream panel component
├── inspector.go      # Job inspector modal component
├── indicators.go     # Ticker, spinner, progress bars
└── client.go         # API client for SSE/health/jobs
```

## Open Questions

1. **Scheduler API:** Does ductile expose scheduled jobs via API? May need to add endpoint.
2. **SSE client count:** Add `sse_clients` field to `/healthz` response?
3. **Config parsing:** Should we read config file directly or add `/pipelines` API endpoint?
4. **Job retention:** How long do job records persist? Do we need local caching?

## Stretch Goals

- **Multiple themes:** Add "blue", "gruvbox", "solarized" presets
- **Config reload:** Watch config file for changes and refresh
- **Filter events:** Regex filter for event stream
- **Export logs:** Save event stream or job details to file
- **Compact mode:** Reduce vertical space for smaller terminals
- **Dashboard mode:** Auto-rotate through pipelines (for wall displays)

## Inspiration Sources

- **systemctl status** - hierarchical service view, live indicators
- **k9s** - multi-panel navigation, live updates, drill-down
- **lazydocker** - compact info density, intuitive keybindings
- **htop** - header stats, color coding, live graphs
- **journalctl -f** - event stream tail with follow mode

---

**Status:** Design approved, ready for implementation
**Next Step:** Phase 1 - Core structure with ticker/spinner/connections
