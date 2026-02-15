---
id: 99
status: todo
priority: Normal
blocked_by: [98]
tags: [tui, monitoring, scheduler, cron]
---

# Overwatch TUI - Scheduler Panel

Add scheduler visibility to `ductile system watch` TUI. Surface cron-like scheduled jobs that often get forgotten during diagnostics.

**Design:** See `/docs/TUI_WATCH_DESIGN.md` Phase 3.

## Motivation

**Problem:** Scheduled poll jobs are invisible in current monitoring
- Operators don't know when next poll is scheduled
- Can't see if scheduler is stuck
- No visibility into job schedules without reading config

**Solution:** Dedicated scheduler panel with:
- Next scheduled job prominently displayed
- Expandable view showing all scheduled jobs
- Last tick timestamp (proves scheduler is alive)
- Per-job schedule, last run, next run

## Acceptance Criteria

### Collapsed View (Default)
```
┌─ SCHEDULED (cron) ─────────────────────────────────────────┐
│ ⏰ Next: 2m - echo/poll (every 5m)                         │
│    Last tick: 6s ago ✓ 6 jobs scheduled                   │
└────────────────────────────────────────────────────────────┘
```
- [ ] Show next scheduled job with countdown
- [ ] Display last scheduler tick timestamp
- [ ] Show total count of scheduled jobs
- [ ] Indicate scheduler health (✓ or ⚠ if stale)

### Expanded View (Press 's' to toggle)
```
┌─ SCHEDULED (cron) ─────────────────────────────────────────┐
│ echo/poll      Every 5m    Last: 3m ago ✓   Next: 2m      │
│ fabric/poll    Every 1h    Last: 12m ago ✓  Next: 48m     │
│ backup/run     0 2 * * *   Last: 13h ago ✓  Next: 11h     │
│ cleanup/run    0 */6 * * * Last: 2h ago ✅  Next: 4h      │
│ health/check   Every 30s   Last: 6s ago ✓   Next: 24s     │
│ report/daily   0 9 * * 1-5 Last: 6h ago ✅  Next: 18h     │
└────────────────────────────────────────────────────────────┘
```
- [ ] Show all scheduled jobs in table format
- [ ] Display schedule (cron or interval format)
- [ ] Show last run timestamp and status
- [ ] Show next run countdown/timestamp
- [ ] Status indicators:
  - [ ] ✓ - Tick ran (scheduler working)
  - [ ] ✅ - Job succeeded
  - [ ] ❌ - Job failed
  - [ ] ⏭ - Job skipped (circuit open, dedupe, etc.)
  - [ ] ⏰ - Scheduled but not run yet

### Keyboard Navigation
- [ ] 's' key toggles scheduler panel collapsed/expanded
- [ ] Panel integrates into existing layout below pipelines

### Integration with Reliability Features
When cards #95-97 land, show additional context:
- [ ] Circuit breaker state affects scheduling (⊘ indicator)
- [ ] Deduplication preventing scheduled runs
- [ ] Poll guard throttling

## Technical Requirements

### Data Sources

**Option 1: Read config directly**
- Parse `plugins[].schedule` from config.yaml
- Calculate next run time locally
- Track last tick from `scheduler.tick` events
- **Pros:** No API changes needed
- **Cons:** Won't show runtime scheduler state

**Option 2: Add /scheduler/jobs endpoint**
- Scheduler exposes its job queue state via API
- Returns: plugin, command, schedule, last_run, next_run, status
- **Pros:** Shows actual scheduler state, not just config
- **Cons:** Requires API implementation

**MVP:** Start with Option 1, upgrade to Option 2 if needed

### Implementation
- Add `scheduler.go` component to `internal/tui/watch/`
- Track `scheduler.tick` events to update "last tick"
- Calculate next run from schedule + last tick
- Toggle state in model with 's' key

## Follow-up Work

After this card:
- **#100** - Job inspector modal and polish

## Narrative
- 2026-02-15: Created to preserve scheduler visibility from TUI_WATCH_DESIGN.md. Scheduled jobs are often forgotten during diagnostics - this surfaces them. (by @claude)
