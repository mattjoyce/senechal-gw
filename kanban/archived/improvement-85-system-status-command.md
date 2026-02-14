---
id: 85
status: done
priority: High
blocked_by: []
tags: [improvement, cli, system, observability]
---

# IMPROVEMENT: Implement `system status` with actionable health output

## Description

`./ductile system status` currently returns a placeholder (`system status is not yet implemented`).
Implement a real status action that gives operators quick, actionable runtime health information.

## Job Story

When I run `ductile system status`, I want to see whether core dependencies and runtime state are healthy, so I can quickly diagnose if the gateway is ready to run.

## Acceptance Criteria

- `ductile system status` returns structured human-readable status output (non-placeholder).
- Command checks and reports:
  - config discovery result/path
  - config load success/failure
  - state DB path and open/readiness result
  - PID lock path and whether another instance appears active
- Exit codes:
  - `0` when all required checks pass
  - `1` when one or more required checks fail
- `ductile system status --json` returns machine-readable output with check results and overall status.
- `ductile system status --help` documents flags, output modes, and exit codes.
- Unit tests cover success and failure paths for both human and JSON output.

## Notes

- Keep checks read-only; `system status` must not mutate config/state.
- Reuse existing config/lock/storage components where possible.

## Narrative
- 2026-02-12: Card created after confirming `system status` is still a placeholder and not useful for operations. (by @assistant)
- 2026-02-13: Implemented `system status` with actionable checks for config discovery/load, database readiness, and PID lock state. Added `--json` machine output, help text with exit codes, and unit tests for healthy status, config-load failure, and active lock detection. (by @assistant)
