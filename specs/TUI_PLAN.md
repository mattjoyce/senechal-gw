# Ductile TUI — Coding Agent Implementation Guide

> Companion to: `Ductile TUI Specification v0.1`
> Stack: Go · Bubble Tea · Lip Gloss · Bubbles
> Status: Agent build reference

---

## 0. Quick orientation

You are building a **terminal operations cockpit** for the Ductile gateway — not a log tail, not a config editor.

The central mental model is **five temporal domains**, with the **live triplet** (Just now → Now → Soon) as the default home screen.

Read the full spec before touching code. This guide translates the spec into concrete build steps, contracts, and gotchas.

**Before writing any code, read these existing files:**

| File | Why |
|------|-----|
| `internal/tui/watch/client.go` | Proven SSE subscription pattern — port directly, do not reimplement |
| `internal/tui/watch/indicators.go` | Proven `Heartbeat` and `Dots` activity indicators — port directly |
| `internal/tui/watch/model.go` | Reference for Bubble Tea model structure and Update routing |

These files contain working, battle-tested code. Reimplementing them is a source of bugs.

---

## 1. Package layout — build this first

```
internal/tui/
  app/           # root model, tab switching, global key map
  screens/
    live/        # Just now + Now + Soon (default home)
    future/      # scheduled/due work
    past/        # aggregate history with drill-down
    structure/   # static system map
    detail/      # universal drill-down target
  components/
    header/      # top status bar
    eventtable/  # reusable scrollable event list
    duelist/     # sorted due-items list
    treetview/   # execution tree renderer
    statusbar/   # footer/help bar
  styles/        # all Lip Gloss styles, single source of truth
  client/        # data access boundary — screens never reach past this
  types/         # shared data types used across screens
```

**Rule:** Screens import `client` and `types`. Screens never import each other. `app` imports screens.

**Critical import constraint:** Message types must live in `internal/tui/msgs/` — NOT in `app/`. If messages are defined in `app/`, any screen that imports them creates a cycle: `app` → `screens/live` → `app`. Add `msgs/` to the package tree above and treat it as a shared dependency with no imports from the rest of the tui tree.

---

## 2. Core data types — define before any screen

Define these in `types/` before writing any screen code. Every screen depends on them.

```go
// RuntimeHealth — for header and Now panel
type RuntimeHealth struct {
    Status       string        // "healthy" | "degraded" | "down"
    Uptime       time.Duration
    WorkerUsage  WorkerUsage
    DBHealthy    bool
}

type WorkerUsage struct {
    Active int
    Total  int
}

// QueueMetrics — for Now panel and header
type QueueMetrics struct {
    Depth       int
    Running     int
    Delayed     int
    Retrying    int
    DeadLetter  int
    OldestAge   time.Duration
}

// Event — for Just now panel and Past screen
type Event struct {
    ID        string
    Timestamp time.Time
    Type      string    // "webhook.received" | "job.started" | "job.failed" etc.
    Subject   string
    JobID     string
    Plugin    string
    Route     string
    Status    string
}

// DueItem — for Soon panel and Future screen
type DueItem struct {
    ID         string
    DueAt      time.Time
    Type       string    // "schedule" | "retry" | "poll" | "delayed"
    Name       string
    Target     string
    Recurrence string    // empty if one-shot
    State      string
}

// AggregateSummary — for Past screen surface
type AggregateSummary struct {
    Window       time.Duration
    Completed    int
    Failed       int
    Retried      int
    AvgDuration  time.Duration
    P95Duration  time.Duration
    ByPlugin     []PluginStat
    ByRoute      []RouteStat
    ErrorGroups  []ErrorGroup
}

// JobDetail — for Detail screen
type JobDetail struct {
    ID              string
    ParentID        string
    RootID          string
    Type            string
    Plugin          string
    Status          string
    Attempts        int
    CreatedAt       time.Time
    StartedAt       *time.Time
    FinishedAt      *time.Time
    SourceEvent     string
    EmittedEvents   []string
    BaggageSummary  map[string]string
    WorkspaceRef    string
    Stderr          string
    Children        []string   // child job IDs
}

// TreeNode — for execution tree view
type TreeNode struct {
    ID       string
    Type     string   // "trigger" | "job" | "event"
    Label    string
    Status   string   // "running" | "queued" | "completed" | "failed" | "delayed" | "waiting"
    Children []TreeNode
}

// StructureData — for Structure screen
type StructureData struct {
    Plugins      []Plugin
    Routes       []Route
    Schedules    []Schedule
    Pollers      []Poller
    Concurrency  []ConcurrencyLimit
}
```

---

## 3. Client interface — implement as a contract

```go
// client/client.go
type Client interface {
    Health(ctx context.Context) (RuntimeHealth, error)
    QueueMetrics(ctx context.Context) (QueueMetrics, error)
    RecentEvents(ctx context.Context, limit int) ([]Event, error)
    DueItems(ctx context.Context, within time.Duration) ([]DueItem, error)
    AggregateSummary(ctx context.Context, window time.Duration) (AggregateSummary, error)
    Structure(ctx context.Context) (StructureData, error)
    JobDetail(ctx context.Context, jobID string) (JobDetail, error)
    ExecutionTree(ctx context.Context, rootID string) (TreeNode, error)
}
```

Provide a `MockClient` that returns realistic static data. Wire this first so screens can be developed independently of any live backend.

**SSE does not belong in this interface.** The `/events` stream is not request/response — it is a long-lived connection that feeds a Bubble Tea command loop. Adding it to `Client` forces an abstraction that makes the goroutine lifecycle impossible to manage correctly. Handle SSE directly in the screen that needs it, using the pattern from `watch/client.go`. See §6a.

---

## 4. Styles — define once, use everywhere

Define all styles in `styles/styles.go`. Screens must not define ad-hoc Lip Gloss styles.

**Required named styles:**

```go
var (
    // Status colours
    StatusHealthy   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))   // green
    StatusRunning   = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))   // cyan
    StatusQueued    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))   // blue
    StatusWaiting   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))   // yellow
    StatusDelayed   = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))   // magenta
    StatusCompleted = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))    // grey
    StatusFailed    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))    // red
    StatusWarning   = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)

    // Panel chrome
    PanelFocused   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12"))
    PanelUnfocused = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))

    // Time display
    TimeRecent  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))   // green — < 30s
    TimeMedium  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))   // yellow — 30s–5m
    TimeOld     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))    // grey — > 5m
    TimeSoon    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))   // cyan — upcoming

    // Tab bar
    TabActive   = lipgloss.NewStyle().Bold(true).Underline(true)
    TabInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

    // Header / summary
    HeaderBar   = lipgloss.NewStyle().Background(lipgloss.Color("235")).Padding(0, 1)
    SummaryKey  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
    SummaryVal  = lipgloss.NewStyle().Bold(true)
)
```

**Time formatting helper — must be used everywhere time is displayed:**

```go
func RelativeTime(t time.Time) string {
    d := time.Since(t)
    switch {
    case d < 0:
        return fmt.Sprintf("in %s", formatDuration(-d))
    case d < time.Minute:
        return fmt.Sprintf("%ds ago", int(d.Seconds()))
    case d < time.Hour:
        return fmt.Sprintf("%dm ago", int(d.Minutes()))
    default:
        return fmt.Sprintf("%dh ago", int(d.Hours()))
    }
}

func TimeUntil(t time.Time) string {
    d := time.Until(t)
    if d <= 0 { return "now" }
    return "in " + formatDuration(d)
}
```

---

## 5. App model — top-level Bubble Tea model

```go
// app/app.go
type Tab int
const (
    TabLive Tab = iota
    TabFuture
    TabPast
    TabStructure
    TabDetail
)

type Model struct {
    width, height int
    activeTab     Tab
    keys          KeyMap
    header        header.Model
    live          live.Model
    future        future.Model
    past          past.Model
    structure     structure.Model
    detail        detail.Model
    client        client.Client
    err           error
}
```

**Critical routing rule:** `app.Update()` must forward ALL unhandled messages to the active screen. Bubble Tea does not do this automatically. Without an explicit `default` case, every data message (ticks, loaded responses, SSE events) is silently discarded and screens never update.

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        // handle global keys ...
    case tea.WindowSizeMsg:
        // handle resize ...
    case msgs.OpenDetailMsg:
        // handle navigation ...
    default:
        // ALL other messages go to the active screen
        return m.updateActiveScreen(msg, nil)
    }
    return m, nil
}
```

**Key map (implement exactly these):**

| Key | Action |
|-----|--------|
| `1`–`5` | Switch to tab 1–5 |
| `tab` / `shift+tab` | Cycle focus regions within current screen |
| `↑↓` or `j k` | Move selection in focused panel |
| `←→` or `h l` | Move selection horizontally (tree, multi-column views) |
| `enter` | Open detail / drill down |
| `esc` | Go back / close detail |
| `/` | Filter within current screen |
| `r` | Force refresh |
| `space` | Freeze / unfreeze live refresh |
| `t` | Open execution tree for selected job |
| `?` | Toggle key help overlay |
| `q` | Quit |

---

## 6. Live screen — build this second (after shell)

### Layout

```
┌─ header ────────────────────────────────────────────────────────────────────┐
│ health: OK  uptime: 4h32m  queue: 3  running: 2  failed (5m): 0  workers: 2/4 │
├─ Just now ──────────────────┬─ Now ───────────────────────────────────────────┤
│ 8s ago  job.completed  ...  │ Queue depth:    3                               │
│ 14s ago job.started    ...  │ Running:        2                               │
│ 32s ago webhook.recv   ...  │ Delayed:        1                               │
│ 1m ago  job.failed     ...  │ Dead letter:    0                               │
│                             │ Oldest queued:  14s                             │
│                             ├─ Plugin lanes ──────────────────────────────────┤
│                             │ fabric    1/1  !!                               │
│                             │ mailer    0/4                                   │
├─ Soon ──────────────────────────────────────────────────────────────────────┤
│ in 4s   retry    job:abc123    fabric                                        │
│ in 12s  schedule hourly-sync   routes/sync                                   │
│ in 1m   poll     github-poller  github                                       │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Refresh

- Live screen: 1s tick
- Header data: every tick
- Just now / Now: every tick
- Soon: every 2s (fewer items change)

### Focus regions

Three focusable regions: `JustNow`, `Now`, `Soon`. Tab cycles between them.

### Selection behaviour

- Selecting an event in Just now or an item in Soon should display a one-line summary at the bottom of the screen.
- `enter` on a job event opens Detail screen.
- `t` on a job event opens execution tree.

---

## 6a. Live screen — SSE subscription and activity indicators

### SSE subscription

The Live screen subscribes directly to `/events` using the pattern from `internal/tui/watch/client.go`. Do not route SSE through the `Client` interface.

Port these two functions verbatim, adapting only the channel element type:

```go
// subscribeToEvents connects to /events, writes to ch, returns SSEDisconnectedMsg when dropped.
// This IS the tea.Cmd goroutine — it runs until the connection drops.
func subscribeToEvents(apiURL, apiKey string, ch chan<- sseEvent) tea.Cmd

// receiveNextEvent blocks on ch for exactly one event, then returns SSEEventMsg.
// Dispatch again after every SSEEventMsg to maintain exactly one receiver in flight.
func receiveNextEvent(ch <-chan sseEvent) tea.Cmd
```

**Reconnect rule (from watch/model.go):**
- On `SSEDisconnectedMsg`: schedule `tea.Tick(3s)` returning a distinct `SSEReconnectMsg`
- On `SSEReconnectMsg`: dispatch `subscribeToEvents()` only — do NOT dispatch a new `receiveNextEvent()` because the existing one is still blocking on the channel
- Never dispatch a new `receiveNextEvent()` except: on `SSEEventMsg` (to receive the next one), and in `Init()` (to start the first one)

Violating these rules causes competing goroutines that split events and starve the Update loop.

### Activity indicators

Do not use `bubbles/spinner`. Port the `Heartbeat` and `Dots` types from `internal/tui/watch/indicators.go` into `internal/tui/components/activity/`. The existing implementation is correct and proven.

- `Dots.OnEvent()` — call on every SSE event; lights up all 5 dots
- `Dots.Decay()` — call on every 1s tick; fades dots over 10s
- `Heartbeat.OnTick()` — call when `event.Type == "scheduler.tick"`; fades ♥ over the tick interval

Both are always rendered in the header — they are never hidden. `Dots` at `○○○○○` indicates silence, not absence.

---

## 7. Future screen

Ordered table. Default sort: due time ascending.

**Columns:** `Due in` · `Type` · `Name` · `Target` · `Cadence` · `Last result`

**Time window presets (keyboard):** `1` = next 5m · `2` = next 30m · `3` = next 2h · `4` = later today

Selecting an item shows a detail pane with: next fire, last result, overlap policy, catch-up policy, route target.

Refresh: 2s.

---

## 8. Past screen

**Default view is aggregate, not raw log.**

Surface layout:
```
Window: [5m] [15m] [60m] [24h]

Completed: 142   Failed: 3   Retried: 7
Avg duration: 340ms   P95: 1.2s

─ By plugin ─────────────────────────────
  fabric    92 ok  2 fail  avg 280ms
  mailer    50 ok  1 fail  avg 410ms

─ Hot routes ──────────────────────────
  webhook → process-order   (41 runs)
  schedule → hourly-sync    (12 runs)

─ Error signatures ────────────────────
  timeout after 30s         (2 occurrences)
  connection refused        (1 occurrence)
```

Drill-down layers (press `enter` on a row to descend):
1. Aggregate summary (surface)
2. Plugin/route/error group detail
3. Individual job runs list
4. Job detail (opens Detail screen)

Refresh: 5s.

---

## 9. Structure screen

Static-ish. Refresh only on entry and on `r`.

Sections (navigable):
- Plugins (name, type, concurrency cap, status)
- Routes (trigger pattern → action)
- Schedules (name, cron/cadence, plugin, next run)
- Pollers (name, interval, plugin)
- Concurrency limits (global + per-plugin)

---

## 10. Detail screen

Universal destination. Reached by `enter` from any screen.

Must handle these targets:
- **Event** — show all event fields
- **Job** — see `JobDetail` type above; include stderr snippet; show child job IDs as navigable list
- **Schedule item** — history, next run, policies
- **Plugin** — capability summary, recent results, lane status
- **Execution tree** — delegates to tree view component

Use a two-pane layout where practical: summary left, content right.

Breadcrumb trail at top showing navigation path (e.g. `Live > Just now > job:abc123`).

---

## 11. Execution tree component

Used from Detail screen and triggered by `t` from Live.

Text tree render, one node per line, indented by depth:

```
● webhook.received (trigger)
  ✓ job:abc123  process-order  fabric        completed  2s
  ✗ job:def456  send-confirm   mailer        failed     retry in 4s
  ◉ job:ghi789  audit-log      audit         running
      ○ job:jkl012  archive    storage       queued
      ✓ job:mno345  index      search        completed  340ms
```

**Node glyphs by status:**
- `◉` running
- `○` queued
- `✓` completed
- `✗` failed
- `⏸` delayed / waiting

`←→` or `h l` to expand/collapse subtrees. `enter` to open node detail.

---

## 12. Error handling in the TUI

All panels must tolerate fetch failures gracefully:

- On error, show stale data with a `[stale]` indicator and error message in the panel chrome border.
- Do not collapse or blank the panel.
- Do not crash or propagate errors to the root model.
- Log errors to a buffer accessible from the detail screen (nice-to-have for v1).

Pattern:
```go
type PanelState struct {
    data      SomeData
    lastFetch time.Time
    fetchErr  error
}

func (p PanelState) isStale() bool {
    return p.fetchErr != nil || time.Since(p.lastFetch) > staleThreshold
}
```

---

## 13. Build order (follow exactly)

| Step | Deliverable | Acceptance signal |
|------|-------------|-------------------|
| 1 | Package skeleton + all `types/` | Compiles with no screens |
| 2 | `client/mock.go` with full `Client` interface | Returns realistic static data |
| 3 | `styles/` package | All status/time/panel styles present |
| 4 | App shell: tab bar, key map, resize, quit | Tabs switch; `?` shows help; `q` quits |
| 5 | Header component wired to mock | Displays all header fields |
| 6 | Live screen (all three panels, mock data) | Triplet visible; tab cycles focus; selection shows summary |
| 7 | Live screen with real client | Refreshes at 1s; stale indicator on error |
| 8 | Future screen | Table renders; time window keys work |
| 9 | Past screen (aggregate surface only) | Summary stats visible; window selector works |
| 10 | Detail screen (job target) | All `JobDetail` fields rendered |
| 11 | Execution tree component | Tree renders; expand/collapse works |
| 12 | Past drill-down (layers 2–4) | `enter` descends; breadcrumb updates |
| 13 | Structure screen | All sections present; manual refresh works |
| 14 | Filter (`/`) on Live and Future | Filters event list and due list |
| 15 | Freeze mode (`space`) | Live screen stops refreshing; resumes on second `space` |
| 16 | Refinement: styling, stale indicators, polish | All acceptance criteria in spec §19 pass |

---

## 14. Bubble Tea message types to define

```go
// Tick messages
type LiveTickMsg struct{}
type FutureTickMsg struct{}
type PastTickMsg struct{}

// Data loaded
type HealthLoadedMsg      struct{ Data types.RuntimeHealth; Err error }
type QueueLoadedMsg       struct{ Data types.QueueMetrics; Err error }
type EventsLoadedMsg      struct{ Data []types.Event; Err error }
type DueItemsLoadedMsg    struct{ Data []types.DueItem; Err error }
type AggregateLoadedMsg   struct{ Data types.AggregateSummary; Err error }
type StructureLoadedMsg   struct{ Data types.StructureData; Err error }
type JobDetailLoadedMsg   struct{ Data types.JobDetail; Err error }
type TreeLoadedMsg        struct{ Data types.TreeNode; Err error }

// Navigation
type OpenDetailMsg  struct{ Target string; ID string }
type OpenTreeMsg    struct{ RootID string }
type NavigateBackMsg struct{}

// UI
type FilterChangedMsg struct{ Query string }
type FreezeToggleMsg  struct{}
type WindowChangedMsg struct{ Width, Height int }
```

---

## 15. v1 acceptance checklist

Before marking v1 complete, verify:

- [ ] Launches reliably in a standard 80×24 terminal and a 220×50 terminal
- [ ] Live screen shows Just now + Now + Soon simultaneously
- [ ] Header shows all seven fields from §6.4
- [ ] Just now events are scrollable and selectable
- [ ] Soon items are sorted by due time with countdown
- [ ] Future screen shows due work with time window filters
- [ ] Past screen opens on aggregate summary, not raw log
- [ ] Detail screen renders all `JobDetail` fields
- [ ] Execution tree renders with correct glyphs and expand/collapse
- [ ] `t` from a selected job event opens the tree
- [ ] `esc` navigates back through drill-down stack
- [ ] Stale data shows `[stale]` indicator without crashing
- [ ] Freeze (`space`) stops and resumes Live refresh
- [ ] `?` shows all active key bindings
- [ ] No screen flickers or full-redraws on normal tick
- [ ] Status colours are consistent with `styles/` package throughout

---

## 16. Backend wiring reference

Resolved answers for step 7:

1. **Endpoints:** `GET /healthz` (no auth), `GET /jobs`, `GET /job/{id}`, `GET /job-logs`, `GET /scheduler/jobs`, `GET /plugins`, `GET /events` (SSE). All authenticated via `Authorization: Bearer <token>` except `/healthz`.
2. **Event streaming:** SSE is available via `GET /events`. Use it. See §6a.
3. **Execution tree root:** root job ID. Use `GET /job/{id}` and follow `parent_job_id` chain, or await a dedicated tree endpoint.
4. **Baggage:** not yet exposed in the API — omit from Detail view for v1.
5. **Future screen:** use `/scheduler/jobs` — shows all scheduled entries with `next_run_at`. Include entries regardless of whether they are due soon.

---

*This guide is the implementation contract. The spec (`Ductile TUI Specification v0.1`) is the authority on intent; this guide is the authority on build order and interface contracts.*
