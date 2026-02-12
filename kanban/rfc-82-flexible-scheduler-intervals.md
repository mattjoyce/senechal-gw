---
id: 82
status: done
priority: Normal
tags: [scheduler, config, enhancement]
---

# RFC: Flexible Scheduler Intervals

The current scheduler is too rigid, supporting only a hardcoded list of intervals (5m, 15m, 30m, hourly, 2h, 6h, daily, weekly, monthly). This prevents users from selecting arbitrary periods like "7m", "13h", or "3d".

## Current Limitation
In `internal/config/loader.go`, the `validate` function explicitly checks `schedule.every` against a fixed whitelist:

```go
validIntervals := []string{"5m", "15m", "30m", "hourly", "2h", "6h", "daily", "weekly", "monthly"}
```

## Proposed Change
Refactor the validation and parsing logic to support:
1.  **Extended Duration Parsing**: Use a community-standard module like `github.com/xhit/go-str2duration`. This library acts as a drop-in replacement for `time.ParseDuration` but adds support for `d` (days) and `w` (weeks).
2.  **Extended Human Aliases**: Support common terms like `daily`, `weekly`, `monthly` by translating them into logical offsets (already partially supported in the scheduler but restricted in the loader).
3.  **Removal of Whitelist**: Eliminate the hardcoded `validIntervals` whitelist in `loader.go`.

## Examples of Desired Config
```yaml
plugins:
  custom-monitor:
    schedule:
      every: 7m      # Arbitrary minutes
  data-backup:
    schedule:
      every: 3d      # Multi-day support
  heavy-task:
    schedule:
      every: 13h     # Prime number hours
```

## Success Criteria
- [x] Remove the hardcoded `validIntervals` whitelist in `loader.go`.
- [x] Update `ParseInterval` to handle standard Go durations and "day" suffixes.
- [x] Ensure the scheduler correctly calculates the next run for arbitrary durations.
- [x] Add unit tests for non-standard intervals (e.g., 7m, 3d, 13h).

## Narrative
- 2026-02-13: Implemented flexible schedule parsing by removing loader whitelist validation and validating `schedule.every` through shared `config.ParseInterval`. Added support for `d` and `w` suffixes (for example `3d`, `2w`) plus existing aliases (`hourly`, `daily`, `weekly`, `monthly`). Scheduler now delegates interval parsing to the same config parser to keep behavior consistent across validation and runtime. Added/updated tests in config and scheduler suites for `7m`, `13h`, `3d`, and related invalid values. (by @assistant)
