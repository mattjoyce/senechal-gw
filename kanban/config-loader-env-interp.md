---
id: 10
status: todo
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

