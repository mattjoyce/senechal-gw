---
id: 124
status: todo
priority: High
blocked_by: []
tags: [improvement, scheduler, ops]
---

# Scheduler: Support Arbitrary Commands (not just poll)

## Problem

`ScheduleConfig` has no `command` field — the scheduler unconditionally calls `poll`
for every scheduled plugin invocation. There is no way to schedule `token_refresh`
or any other non-`poll` command independently.

This makes it impossible to have different cadences for different concerns within the
same plugin. Example: Withings needs token refresh every hour and data fetch every 10
minutes — two separate commands, two separate schedules. Currently forced into a single
`poll` that combines both.

## Proposed Solution

Add optional `command` and `payload` fields to `ScheduleConfig` (command defaults
to `poll` for backwards compatibility):

```yaml
# config/types.go
type ScheduleConfig struct {
  Every   string         `yaml:"every"`
  Jitter  time.Duration  `yaml:"jitter,omitempty"`
  Command string         `yaml:"command,omitempty"` // default: "poll"
  Payload map[string]any `yaml:"payload,omitempty"` // optional, default: {}
}
```

Allow multiple schedule entries per plugin:

```yaml
withings:
  enabled: true
  schedules:
    - command: token_refresh
      every: 1h
      jitter: 5m
    - command: poll
      every: 15m
      jitter: 2m
```

Or single schedule with explicit command:

```yaml
withings:
  enabled: true
  schedule:
    command: token_refresh
    every: 1h
    jitter: 5m
    payload:
      refresh: true
```

## Acceptance Criteria

- [ ] `ScheduleConfig` has optional `command` field, defaults to `poll`
- [ ] `ScheduleConfig` supports optional `payload` (defaults to empty object)
- [ ] Plugin config supports `schedules` (plural) as a list, each with its own command + cadence
- [ ] Backwards compatible — existing `schedule:` with no `command` still calls `poll`
- [ ] Scheduled `handle` is explicitly rejected at config load/reload and schedule submission
- [ ] Scheduled command must exist in plugin manifest; unknown command rejects config/submission
- [ ] If command has `input_schema`, configured `payload` must validate (reject invalid schedule config/submission)
- [ ] Scheduler fires the correct command at the correct interval
- [ ] Deduplication key includes plugin + command + schedule identity (no cross-schedule suppression)
- [ ] Circuit breaker and outstanding-guard behavior are keyed by `(plugin, command)` for scheduler jobs

## Motivation

Discovered during ductile-withings deployment (2026-02-22). The withings plugin has
`token_refresh` and `poll` as distinct commands requiring different schedules. Without
this feature, both must be collapsed into a single `poll` call.

## Defaults

- `command` default: `poll`
- `payload` default: `{}`
- Misfire policy default: `skip` (no backfill flood after downtime/restart)
- Scheduler protection scope: scheduler-originated jobs only (API/webhook triggers are not blocked by scheduler breaker state)

## Edge Cases

- Multiple schedules for the same `(plugin, command)` with different payloads must remain independent.
- Config reload must be atomic: scheduler ticks use a consistent config snapshot.
- Runtime drift (plugin command removed after a valid config load) should fail terminally without retry and mark schedule entry invalid until fixed.

## Decisions

- `schedule` + `schedules` together: **Reject config** with clear validation error.
- Schedule identity: **Require explicit `id`** per schedule entry.
- Dedupe identity: **Use `plugin:command:schedule_id`** (do not include payload hash for MVP).
- Schedule-entry state persistence: **SQLite table** keyed by `(plugin, schedule_id)` for durable `active|paused_manual|paused_invalid` state.

## Narrative
- 2026-02-22: Clarified that `handle` is not schedulable, added schedule payload support for command-specific input, and pinned defaults/edge-case behavior to reduce implementation ambiguity. (by @assistant)
- 2026-02-22: Locked decisions to reject mixed `schedule`/`schedules`, require explicit schedule IDs, and use dedupe key `plugin:command:schedule_id`; left schedule-state persistence as the remaining design choice. (by @assistant)
- 2026-02-22: Finalized schedule-entry state persistence to SQLite for durable pause/invalid behavior across restarts. (by @assistant)
