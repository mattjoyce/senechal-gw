---
id: 100
status: todo
priority: Normal
blocked_by: [98, 99]
tags: [tui, monitoring, diagnostics, ux, polish]
---

# Overwatch TUI - Job Inspector & Polish

Add deep-dive job inspector modal and visual polish to `ductile system watch` TUI.

**Design:** See `/docs/TUI_WATCH_DESIGN.md` Phase 5 + 6.

## Job Inspector (Phase 5)

**Motivation:** When diagnosing issues, operators need to see:
- Full job execution timeline
- Input payload that triggered the job
- Plugin logs and error messages
- Parent/child job relationships

**Trigger:** Press 'j' on selected job, or click job ID in event stream

### Modal View
```
┌─ JOB INSPECTOR: 09fbb6e6 ─────────────────────────────────┐
│ Pipeline: discord-fabric                                  │
│ Status: ◉ RUNNING (45.2s / 60s timeout)                  │
│ Started: 2024-02-15 15:23:43                              │
│                                                           │
│ ┌─ TIMELINE ─────────────────────────────────────────────┐│
│ │ 15:23:43  job.enqueued      → Queue depth: 1         ││
│ │ 15:23:43  job.started       → fabric.handle          ││
│ │ 15:23:44  fabric.completed  → fabric plugin          ││
│ │ 15:23:45  [current]         → Waiting for LLM...     ││
│ └───────────────────────────────────────────────────────┘│
│                                                           │
│ ┌─ PAYLOAD ──────────────────────────────────────────────┐│
│ │ {                                                     ││
│ │   "prompt": "What does ductile mean?",               ││
│ │   "pattern": "",                                     ││
│ │   "model": "gpt-4"                                   ││
│ │ }                                                     ││
│ └───────────────────────────────────────────────────────┘│
│                                                           │
│ ┌─ LOGS ─────────────────────────────────────────────────┐│
│ │ [INFO] fabric: Executing prompt-only mode           ││
│ │ [INFO] fabric: Calling OpenAI API...                ││
│ └───────────────────────────────────────────────────────┘│
│                                                           │
│ [ESC] Close • [↑/↓] Scroll                               │
└───────────────────────────────────────────────────────────┘
```

### Acceptance Criteria
- [ ] Modal overlay on top of main view
- [ ] Fetch job details from `/jobs/:id` API
- [ ] Display sections:
  - [ ] Job metadata (pipeline, status, timestamps)
  - [ ] Timeline of events for this job
  - [ ] Input payload (formatted JSON)
  - [ ] Plugin logs (if available)
  - [ ] Parent/child relationships (if applicable)
- [ ] Scrollable content (↑/↓ keys)
- [ ] Close with ESC key
- [ ] Handle API errors gracefully

### Integration with Reliability Features
Show additional context when available:
- [ ] Retry history (#96): "Attempt 2/3, retrying in 8s"
- [ ] Circuit breaker (#97): "Plugin circuit: open"
- [ ] Deduplication (#95): "Duplicate of job abc123"

## Visual Polish (Phase 6)

### Progress Bars
For running jobs with known timeouts:
```
◉ 45.2s / 60s  ████████████░░░░░░░  75%
```
- [ ] Show progress bar in pipelines panel
- [ ] Calculate percentage: `(elapsed / timeout) * 100`
- [ ] Color coding: green → yellow → red as timeout approaches
- [ ] Handle jobs without timeout (show spinner only)

### Retry Indicators
For jobs with retry metadata (#96):
```
Job abc123 (attempt 2/3, retry in 4s) ⟲
```
- [ ] Show attempt count inline
- [ ] Show countdown to next retry
- [ ] Use retry spinner indicator

### Circuit Breaker Indicators
When circuit is open (#97):
```
fabric/poll - ⊘ Circuit open, retry in 5m
```
- [ ] Show ⊘ symbol for open circuits
- [ ] Display cooldown countdown
- [ ] Gray out affected scheduled jobs

### Help Overlay
Press '?' to show keyboard shortcuts:
```
┌─ KEYBOARD SHORTCUTS ───────────────────────────────────────┐
│ q, Ctrl+C    Quit                                          │
│ ↑/↓          Navigate pipelines                            │
│ →/←          Expand/collapse pipeline                      │
│ s            Toggle scheduler panel                        │
│ j            Open job inspector                            │
│ f            Toggle event follow mode                      │
│ ?            Show this help                                │
└────────────────────────────────────────────────────────────┘
```
- [ ] Help modal triggered by '?' key
- [ ] Shows all available keybindings
- [ ] Close with '?' or ESC

### Edge Cases & Polish
- [ ] Handle terminal resize gracefully
- [ ] Reconnect SSE on disconnect (show warning)
- [ ] Handle empty states:
  - [ ] No active pipelines
  - [ ] No scheduled jobs
  - [ ] No recent events
- [ ] Loading states for API calls
- [ ] Error messages for API failures
- [ ] Truncate long job IDs/names intelligently

## Stretch Goals

If time permits:
- [ ] Click job ID to inspect (mouse support)
- [ ] Export job details to JSON file
- [ ] Filter event stream by type/plugin
- [ ] Compact mode for smaller terminals
- [ ] Multiple theme support (load from config)

## Technical Requirements

### API Dependencies
- `/jobs/:id` - Fetch job details (timeline, payload, logs)
- Ensure timeline includes all job.* events for the job
- Ensure logs are captured and queryable

### Implementation
- Add `inspector.go` modal component
- Add `indicators.go` for progress bars/spinners
- Update `model.go` with modal state management
- Add keyboard handler for 'j' and '?' keys

## Follow-up Work

After this ships, Overwatch is feature-complete per design spec!

Optional future enhancements could be new cards:
- Theme customization from config
- Export/logging features
- Performance optimizations for large job counts

## Narrative
- 2026-02-15: Created to preserve job inspector and polish phases from TUI_WATCH_DESIGN.md. This completes the full diagnostic tool vision. (by @claude)
