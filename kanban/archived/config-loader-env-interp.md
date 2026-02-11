---
id: 10
status: done
priority: High
blocked_by: [9]
tags: [sprint-1, mvp, config]
---

# Config Loader + Env Interpolation

Implement `config.yaml` parsing and `${ENV_VAR}` interpolation so operators can keep secrets out of config files while preserving a simple YAML workflow.

## Acceptance Criteria
- Parse `config.yaml` fields needed by MVP: `service`, `state.path`, `plugins_dir`, and a single `plugins.<name>` entry.
- Interpolate `${ENV_VAR}` syntax in string values.
- Validate required plugin fields for MVP: `enabled`, `schedule.every`, `schedule.jitter`, and `timeouts.poll` (or defaults applied).
- Provide clear validation errors (file/field) and fail fast on invalid config.

## Narrative
- **2026-02-08: Complete.** Implemented `internal/config` package with:
  - `types.go`: Complete config types matching SPEC §11, with sensible defaults
  - `loader.go`: YAML parsing, `${ENV_VAR}` interpolation (regex-based), validation, plugin defaults merging
  - `loader_test.go`: Table-driven tests for loading, interpolation, interval parsing, validation (all passing)
  - Security: Fails fast on unresolved env vars in plugin config (prevents secret leakage)
  - MVP subset: Validates schedule.every against allowed values (5m, 15m, 30m, hourly, 2h, 6h, daily, weekly, monthly)

