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
for every scheduled plugin invocation. There is no way to schedule `token_refresh`,
`handle`, or any other command independently.

This makes it impossible to have different cadences for different concerns within the
same plugin. Example: Withings needs token refresh every hour and data fetch every 10
minutes — two separate commands, two separate schedules. Currently forced into a single
`poll` that combines both.

## Proposed Solution

Add an optional `command` field to `ScheduleConfig` (defaults to `poll` for
backwards compatibility):

```yaml
# config/types.go
type ScheduleConfig struct {
  Every   string        `yaml:"every"`
  Jitter  time.Duration `yaml:"jitter,omitempty"`
  Command string        `yaml:"command,omitempty"` // default: "poll"
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
```

## Acceptance Criteria

- [ ] `ScheduleConfig` has optional `command` field, defaults to `poll`
- [ ] Plugin config supports `schedules` (plural) as a list, each with its own command + cadence
- [ ] Backwards compatible — existing `schedule:` with no `command` still calls `poll`
- [ ] Scheduler fires the correct command at the correct interval
- [ ] Deduplication key includes command name (e.g. `token_refresh:withings` not `poll:withings`)

## Motivation

Discovered during ductile-withings deployment (2026-02-22). The withings plugin has
`token_refresh` and `poll` as distinct commands requiring different schedules. Without
this feature, both must be collapsed into a single `poll` call.
