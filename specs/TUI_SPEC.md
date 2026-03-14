# CLAUDE.md — Ductile
# Ductile TUI Specification

Status: Draft v0.2
Target stack: Go, Bubble Tea, Lip Gloss
Primary audience: coding agents and maintainers implementing the Ductile terminal UI

---

## 1. Purpose

This document specifies the first serious Terminal UI (TUI) for Ductile.

The TUI is **not** primarily a configuration tool and **not** merely a log tail. It is an **operator observability interface** for watching the gateway, understanding what is happening, what has recently happened, and what is expected to happen next.

The TUI should help an operator answer these questions quickly:

* Is the gateway healthy?
* What is happening right now?
* What just happened that explains the current state?
* What is about to happen next?
* What is scheduled for later?
* Where are the bottlenecks?
* What failed?
* What tree of work was spawned from a trigger?
* If something seems wrong, where do I drill in?

---

## 2. Design Philosophy

### 2.1 The TUI is an operations cockpit

The TUI should present Ductile as a living runtime, not a static config browser.

It should emphasize:

* flow over inventory
* transitions over static objects
* current relevance over exhaustive history
* drill-down over clutter
* clear temporal framing

### 2.2 Temporal observatory

The TUI should be designed around **temporal domains**.

Ductile exposes information across five temporal domains:

1. **Before just now** — compressed past; aggregate history and trends
2. **Just now** — recent evidence; specific events that explain the present
3. **Now** — current state
4. **Soon** — imminent work; things due shortly
5. **Future** — later scheduled/expected work

These domains are not equally weighted.

### 2.3 The phase transition band

The central live operator view is the triplet:

* **Just now**
* **Now**
* **Soon**

This triplet is the system's **phase transition band**.

It is the zone where:

* recent causes remain explanatory
* current state is still active and unstable
* near-future work is already exerting operational force

This triplet should form the default live screen.

### 2.4 Asymmetry of temporal domains

* **Before just now** and **Future** can stand alone as screens or tabs.
* **Now** should not stand alone. It should be shown with **Just now**, **Soon**, or both.
* The default home screen should be **Just now + Now + Soon**.

### 2.5 Tree, not pipe

Ductile work propagation should be presented conceptually as a **tree**, not just as a linear pipeline.

A root trigger may spawn jobs, which may emit more events, which may spawn more jobs. The operator should be able to inspect:

* roots
* branches
* descendants
* branch status
* failures within subtrees
* running descendants

The TUI should therefore support a **lineage/execution tree** view.

---

## 3. Information Forms

The TUI should model information in the following forms:

### 3.1 State

Current condition of the system.

Examples:

* queue depth
* running jobs
* worker usage
* plugin lane saturation
* health indicators

### 3.2 Evidence

Recent specific events that explain current state.

Examples:

* webhook received
* job started
* job failed
* retry scheduled
* route emitted child jobs

### 3.3 Expectation

Known or inferred near-future work.

Examples:

* next scheduled run
* poll due soon
* retry due soon
* delayed continuation

### 3.4 Aggregate memory

Compressed summaries of past behaviour.

Examples:

* failure counts
* route frequency
* queue trend
* plugin success ratio
* average durations

### 3.5 Structure

Stable map of the system.

Examples:

* plugins
* routes
* schedules
* concurrency caps
* configured pollers

### 3.6 Drill-down / trace

Detailed exploration of a selected object.

Examples:

* job detail
* stderr
* baggage
* workspace lineage
* execution tree
* child jobs
* route trace

---

## 4. Product Scope

### 4.1 In scope for v1

* live triplet screen
* future/schedule screen
* past/aggregate screen
* structure screen
* job detail screen
* execution tree / lineage drill-down
* keyboard-driven navigation
* polling/refresh model suitable for a live terminal app
* clear visual hierarchy via Lip Gloss styles

### 4.2 Out of scope for v1

* full configuration editing
* in-TUI route/plugin authoring
* rich charting libraries beyond text-first rendering
* full log explorer replacement
* mouse-first interactions
* distributed multi-node federation

---

## 5. Screen Model

The TUI should have primary screens (tabs/views) and secondary drill-down modes.

Recommended top-level tabs:

1. **Live**
2. **Future**
3. **Past**
4. **Structure**
5. **Detail**

A sixth optional top-level tab may later exist for **Alerts** if needed, but v1 can incorporate alerting summaries inside Live and Past.

---

## 6. Live Screen

### 6.1 Purpose

The Live screen is the default home screen.

It should show the phase transition band:

* **Just now** — recent evidence/events
* **Now** — current state/health
* **Soon** — imminent due work

### 6.2 Primary questions answered

* What is happening right now?
* What just caused the present condition?
* What is about to happen next?
* Is anything unhealthy or bottlenecked?

### 6.3 Layout

Suggested layout:

* top status bar / header
* left panel: Just now
* center/right panel: Now
* bottom band or side panel: Soon
* lower detail strip for selected item summary

A simple first implementation can use a two-column top area with a bottom full-width "Soon" band.

### 6.4 Live header contents

The header is **two lines**, always visible across all screens.

**Line 1 — operational summary (full width):**

```
⟲ ♥ DUCTILE WATCH  ✅ HEALTHY  ⏱ 3h 33m  Queue: 0  Req: 12/m  Clients: 1  Last: 5s ●●●○○   22:23:10
```

Fields left-to-right:
* **Ticker** (`⟲`, local-tick driven): Rotates every 1s to prove the TUI process hasn't frozen.
* **Heartbeat** (`♥`, SSE-driven): Fades out between scheduler ticks; proof the *backend* is pulsing.
* title: `DUCTILE WATCH`
* health icon + status text (`✅ HEALTHY` / `🔌 CONNECTING` / `⚠️ DEGRADED`)
* uptime
* queue depth
* **API Throughput** (`Req: N/m`): Recent request frequency.
* **Connection Count** (`Clients: N`): Number of active SSE subscribers (including this TUI).
* last event age + **Activity Dots** (`●●●○○`, SSE-driven): Rotates on every incoming event.
* clock (right-aligned)

**Line 2 — static metadata (dimmed):**

```
Config: /home/matt/.config/ductile/  |  Bin: /home/matt/.local/bin/ductile  |  Version: 1.0.0-rc.1  |  API: http://localhost:8081
```

Fields: config path, binary path, version, API URL. Truncated at terminal width. Dimmed style.

This replaces the previous four-line header design. The two-line form keeps the header compact while retaining all operational and static fields.

### 6.5 Just now panel contents

This panel shows recent events as evidence.

Examples:

* webhook received
* route matched
* job queued
* job started
* job completed
* job failed
* retry scheduled
* branch emitted

Rules:

* reverse chronological or newest-near-bottom chronological display is acceptable; choose one and keep it consistent
* should be scrollable
* each row should have timestamp/age, type, concise subject, and short status
* selecting an event should populate the detail panel

### 6.6 Now panel contents

This panel shows current state and **active work in progress**. It must be focusable and selectable — not a read-only status display.

Primary content: **worker/concurrency slots**, each showing the job currently occupying it. Render one row per slot:

```
Worker 1  ◉ job:abc123  fabric › process-order   14s
Worker 2  ◉ job:def456  mailer › send-confirm      2s
Worker 3  ○ (idle)
Worker 4  ○ (idle)
```

Each occupied slot shows: worker index, job ID, plugin, command/pipeline label, elapsed time. Idle slots are shown explicitly so the operator sees total concurrency capacity at a glance.

Per-plugin lane concurrency is shown beneath the global worker list when plugins have their own caps:

```
fabric    1/1  !!   (saturated)
mailer    1/4
```

Selecting any occupied worker slot allows `enter` to open the job in Detail and `t` to open its execution tree.

Secondary content (summary band, below workers):

* queue depth
* delayed jobs
* retrying jobs
* dead-letter count
* current alerts e.g. `queue age rising`

If all slots are idle, show `(idle)` prominently with queue depth.

### 6.7 Soon panel contents

This panel shows imminent due work.

Examples:

* next poll fires
* retry due soon
* schedule due soon
* delayed continuation due soon
* waiting callback due or expiring soon

Rules:

* sort by due time ascending
* display countdown or time-to-due
* allow item inspection
* may support filtering by type (schedule, retry, poll, delayed)

### 6.8 Live screen interaction goals

The operator should be able to:

* move selection between panels
* inspect selected item details
* open job detail
* open execution tree
* filter current view
* pause/resume auto-refresh if needed

---

## 7. Future Screen

### 7.1 Purpose

The Future screen gives later scheduled and expected work a first-class home.

This screen exists because schedule visibility is important and should not be awkwardly squeezed into the live screen.

### 7.2 Primary questions answered

* What should happen later?
* What is due next?
* What recurring schedules exist?
* What retries/delayed tasks are pending?

### 7.3 Content model

The Future screen should show an ordered list/table of due items.

Types may include:

* schedule
* retry
* poll
* delayed resume
* maintenance/cleanup task

Suggested columns:

* due in / due at
* type
* name/target
* frequency or cadence
* status

### 7.4 Detail pane

Selecting an item should show details such as:

* next fire time
* last run result
* overlap policy
* catch-up policy
* route target
* emitted event type

### 7.5 Future horizon windows

Support time window presets, for example:

* next 5 minutes
* next 30 minutes
* next 2 hours
* later today

This can be implemented as simple keyboard filters.

---

## 8. Past Screen

### 8.1 Purpose

The Past screen is the compressed memory of the gateway.

It should begin aggregated, not as raw history.

### 8.2 Primary questions answered

* What kind of behaviour has the system shown recently?
* Where have failures clustered?
* Which routes/plugins are hot?
* Is queue health improving or degrading?

### 8.3 Surface model

The default Past screen should present summaries such as:

* completed / failed / retried counts in a time window
* average and p95 runtime
* failure counts by plugin
* queue trend
* hot routes
* common error signatures

### 8.4 Drill-down model

Past should be explorable in layers:

1. aggregate summaries
2. grouped buckets (by plugin, route, time bucket, error type)
3. individual runs/events
4. item detail / trace

This means Past is:

* aggregate at the surface
* narrative when opened
* forensic when drilled deeply

### 8.5 Time windows

Provide selectable windows, e.g.:

* last 5m
* last 15m
* last 60m
* last 24h

---

## 9. Structure Screen

### 9.1 Purpose

The Structure screen shows the stable map of the machine.

It is less about live watching and more about orientation.

### 9.2 Primary questions answered

* What plugins exist?
* What routes are configured?
* What schedules exist?
* What concurrency limits are in force?
* What pollers are defined?

### 9.3 Suggested sections

* plugin list with metadata
* route list
* schedule list
* concurrency caps
* topology summary

### 9.4 Optional later enhancements

* route graph visualization
* plugin capability summary
* status overlays on structure objects

---

## 10. Detail View

### 10.1 Purpose

The Detail view is a universal drill-down destination from any screen.

### 10.2 Supported detail targets

* event
* job
* schedule item
* plugin
* route
* subtree/root execution

### 10.3 Job detail contents

Recommended fields:

* job ID
* parent/root ID
* type
* plugin
* status
* attempts
* timing
* **Timeline (Evidence):** A vertical list of events specific to this job (enqueued -> started -> etc).
* source trigger
* emitted events
* baggage summary (omit v1)
* workspace reference
* stderr snippet or full stderr view
* linked descendants/children

---

## 11. Execution Tree / Lineage View

### 11.1 Purpose

Ductile work should be viewable as a tree of propagation.

This may be a mode of Detail or a dedicated sub-view.

### 11.2 Questions answered

* What started this work?
* What child jobs were spawned?
* Which branches completed?
* Which branches failed?
* Which branches are still running?
* Is any subtree stuck or waiting?

### 11.3 Rendering

A simple text tree is sufficient for v1.

Example shape:

* root trigger

  * job A completed
  * job B failed (retry due)
  * job C running

    * child C1 queued
    * child C2 completed

### 11.4 Visual cues

Use style to indicate node state:

* running
* queued
* completed
* failed
* delayed
* waiting

### 11.5 Interaction

The operator should be able to:

* expand/collapse nodes
* move through nodes
* open selected node detail

---

## 12. Data Requirements

The TUI needs a backend access model. Exact implementation may vary, but the following data classes are required.

### 12.1 Runtime health

* health status
* uptime
* worker pool usage
* database/connectivity health

### 12.2 Queue metrics & State Aggregation

The Now panel and Header require high-frequency state metrics. Raw polling of the `/jobs` endpoint is inefficient for these.

**Required Metrics:**
- queue depth
- running count
- delayed count
- retry count
- dead-letter count
- oldest queued age
- worker usage (active/total)
- plugin lane occupancy (active/limit)

### 12.3 Event feed

Recent event stream with enough information to render evidence rows.

Minimum fields:

* timestamp
* event type
* subject
* related job ID
* related plugin/route
* status/result

### 12.4 Due items

For Soon and Future.

Minimum fields:

* due time
* type
* name
* target
* state
* recurrence/cadence if applicable

### 12.5 Backend Aggregation (Memory)

The Past screen starts with aggregate summaries. The backend MUST provide an aggregation endpoint (e.g., `GET /analytics/summary?window=1h`) to avoid the TUI client having to download and process thousands of raw log entries locally.

**Required Aggregates:**
- counts by status (completed, failed, retried)
- counts by plugin
- counts by route
- duration stats (Avg, P95)
- error signature groups (common failure messages)

### 12.6 Structure data

* plugins
* routes
* schedules
* pollers
* concurrency limits

### 12.7 Detail/trace data

* job details
* event lineage
* parent/child relationships
* subtree relationships
* stderr/log excerpts
* baggage summary
* workspace reference

---

## 13. Update and Refresh Model

### 13.1 General guidance

The TUI should feel live, but should not flicker excessively or redraw more than necessary.

### 13.2 Refresh cadence

Suggested initial approach:

* periodic refresh tick for summary data
* faster cadence for Live screen than for Past/Future/Structure
* only refresh visible screens unless background prefetch is helpful

Initial simple defaults may be:

* Live: 1s refresh
* Future: 2s refresh
* Past: 5s refresh
* Structure: on entry/manual refresh

These numbers are provisional.

### 13.3 Event-driven updates

If Ductile later exposes push/event streams, the TUI may adopt a more event-driven update model. v1 may use polling.

### 13.4 Pause/freeze

Support a temporary freeze of the display while allowing navigation/inspection. This is useful when a fast-updating screen makes detail reading difficult.

---

## 14. Interaction Model

### 14.1 Navigation

Suggested keys:

* `1..5` switch tabs
* `tab` / `shift+tab` cycle focus regions
* arrow keys or `hjkl` move selection
* `enter` open detail/drill-down
* `esc` go back
* `/` filter/search within current screen
* `r` refresh
* `space` freeze/unfreeze live refresh
* `t` open tree view for selected job
* `?` help
* `q` quit

### 14.2 Focus model

Only one region should be focused at a time.
Focused region should have a clear style distinction.

### 14.3 Selection persistence

When possible, retain selection after refresh if the selected item still exists.

---

## 15. Visual Design Guidance

### 15.1 Libraries

Use:

* Bubble Tea for state/update loop
* Lip Gloss for layout and styling
* Bubbles components selectively where useful (tables, viewport, help, key maps, lists)

### 15.2 Style goals

The UI should be:

* calm
* readable
* information-dense but not cluttered
* clearly hierarchical
* terminal-native

### 15.3 Status styling (The 8-Step Traffic Light)

Use an interpolated 8-step palette to signal operational urgency:

*   **Healthy / Success:** `#50FA7B` (Vibrant Green)
*   **Running / Active:** `#8BE9FD` (Cyan)
*   **Queued / New:** `#BD93F9` (Light Purple)
*   **Waiting / Idle:** `#F1FA8C` (Pale Yellow)
*   **Delayed / Retrying:** `#FFB86C` (Soft Orange)
*   **Warning / Saturated:** `#FF922B` (Amber)
*   **Inactive / Dead:** `#6272A4` (Muted Blue-Grey)
*   **Failure / Critical:** `#FF5555` (Vibrant Red)

### 15.4 UI Identity (Blue-Orange-Purple)

The structural chrome of the application follows a specific visual identity:

*   **Tabs & Headers:** `#61AFEF` (Steel Blue)
*   **Focused Borders:** `#C678DD` (Deep Purple)
*   **Soon Domain Accents:** `#D19A66` (Warm Orange)
*   **Background Surfaces:** `#282C34` (Deep Slate)
*   **Heartbeat Pulse:** `#FF79C6` (Hot Pink)
*   **Activity Indicators:** `#FFFFFF` (Stark White)

### 15.5 Density by screen

Different screens should use different density profiles:

* Live: selective and high-signal
* Future: orderly list/table density
* Past: summary-heavy with drill-down
* Structure: map-like clarity

---

## 16. Architecture Guidance for Implementation

### 16.1 Model decomposition

Recommended high-level Bubble Tea model decomposition:

* app model

  * active tab
  * shared data cache
  * key map
  * status/header model
  * per-screen model
  * detail/overlay model

Per-screen models:

* live model
* future model
* past model
* structure model
* detail model

### 16.2 Data access boundary

Prefer a clean client/service boundary so screen models do not know how transport works.

Example conceptual packages:

* `internal/tui/app`
* `internal/tui/screens/live`
* `internal/tui/screens/future`
* `internal/tui/screens/past`
* `internal/tui/screens/structure`
* `internal/tui/screens/detail`
* `internal/tui/components/...`
* `internal/tui/styles`
* `internal/tui/client`
* `internal/tui/types`

### 16.3 Messages and commands

Use Bubble Tea messages for:

* refresh ticks
* data loaded responses
* navigation actions
* filter actions
* detail-open events
* resize events
* refresh errors

### 16.4 Rendering discipline

Avoid monolithic `View()` methods. Prefer composable sub-renderers for:

* header
* panel chrome
* list/table body
* summary strip
* footer/help

---

## 17. Error Handling

The TUI should fail gracefully when data cannot be fetched.

Examples:

* backend unavailable
* partial data unavailable
* slow responses
* malformed detail payload

Guidance:

* keep app responsive
* display stale-data indicators where appropriate
* show fetch errors in a visible but non-destructive way
* avoid collapsing the whole screen for one failed panel

---

## 18. v1 Priorities

Recommended implementation order:

1. app shell, tab switching, header, key help
2. Live screen with mocked/static data
3. backend client abstraction
4. Live screen wired to real data
5. Future screen
6. Past aggregate screen
7. Detail view
8. Execution tree view
9. Structure screen
10. refinement, styling, filter/search, freeze mode

---

## 19. Acceptance Criteria for v1

The v1 TUI is acceptable when:

* it launches reliably in a normal terminal
* the Live screen clearly shows Just now + Now + Soon
* the Future screen gives schedules and due work a clear home
* the Past screen starts with aggregates, not raw logs
* an operator can drill into a selected job
* an operator can inspect an execution tree/lineage for a job/root
* screen focus and navigation are clear and usable
* refresh behaviour is stable and not visually noisy
* styling clearly distinguishes normal, warning, and failure states

---

## 19b. Resolved Design Decisions

These decisions were debated and resolved during design review. Do not re-open them without a compelling reason.

### Data access

**Shared data cache in app model.**
Screens do not call `client` directly. The `app` model owns a `DataCache` struct with timestamps and dispatches fetches. Screens emit `RequestDataMsg` types and receive `*LoadedMsg` responses. This prevents redundant N fetches per tick when multiple screens need overlapping data (e.g. `Health` for the header) and enables stale-data indicators to be handled centrally.

**SSE feeds Live; REST feeds everything else.**
The boundary is explicit. `Just now` panel = SSE event log (live, ephemeral). Past screen = SQL job records via REST. SSE is not routed through the `Client` interface — it is handled directly in the Live screen using the proven pattern from `watch/client.go`.

**`ListJobs` must be in the Client interface.**
`AggregateSummary` alone cannot support Past drill-down layer 3 (group → individual runs). Add:
```go
ListJobs(ctx context.Context, filter JobFilter) ([]JobSummary, error)
```
where `JobFilter` holds `plugin`, `route`, `errorSignature`, `window`, `limit`. Without this, Past drill-down silently dead-ends.

**`GET /job/{id}/tree` endpoint required.**
Walking `parent_job_id` chains client-side costs O(depth) sequential round-trips. A server-side tree endpoint is required. The chain-walking fallback must not be implemented as it will become the permanent solution by default.

### Message types

**`msgs/` contains only data-loaded and tick messages.**
Navigation messages such as `OpenDetailMsg` must not encode knowledge of detail target types in `msgs/`. Use `types.DetailTarget` (an interface defined in `types/`) as the payload. This prevents `msgs/` from coupling to the full set of navigable targets.

### Focus model

**`Now` panel is focusable and selectable.**
Now shows worker/concurrency slots as the primary view — one row per slot, showing the job currently occupying it (or idle). This makes total concurrency capacity visible at a glance. Per-plugin lane saturation is shown beneath. Selecting an occupied slot allows `enter` → Detail and `t` → execution tree. Queue summary (depth, delayed, dead-letter) appears as a secondary band below workers.

### Refresh and SSE interaction

**SSE events trigger immediate `QueueMetrics` fetch.**
On receipt of any SSE event in the Live screen, dispatch an immediate `QueueMetrics` fetch rather than waiting for the next 1s tick. This keeps the Now panel responsive to actual events, not just to clock ticks.

### Freeze mode

**Freeze semantics:** data fetches stop; SSE connection stays open; incoming SSE events queue in memory; display does not update. On unfreeze, flush queued events and trigger an immediate full refresh. This is the most useful semantics for debugging a fast-moving screen.

---

## 20. Open Questions

These should be resolved during implementation design:

* What concrete backend endpoints or internal APIs will the TUI consume?
* Will event streaming be available, or polling only?
* What is the canonical identifier for a root execution tree?
* How should baggage/workspace data be summarized in a concise but useful way?
* Should Future include only due work or also all configured schedules?
* Should alerts become a dedicated screen later?
* How much historical retention is practical in the TUI client?

---

## 21. Summary

The Ductile TUI should be built as a temporal operations cockpit.

Core ideas:

* five temporal domains
* a central live triplet as the phase transition band
* Future as first-class schedule visibility
* Past as aggregate-first but explorable
* execution viewed as a tree, not merely a pipe
* strong drill-down and lineage inspection

This specification is the conceptual basis for implementation in Go using Bubble Tea and Lip Gloss.
