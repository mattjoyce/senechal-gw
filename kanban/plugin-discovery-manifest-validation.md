---
id: 12
status: todo
priority: High
blocked_by: [9, 10]
tags: [sprint-1, mvp, plugins]
---

# Plugin Discovery + Manifest Validation

Scan `plugins_dir` for plugins with `manifest.yaml` and validate they are runnable and compatible with protocol v1.

## Acceptance Criteria
- Discover plugin directories containing `manifest.yaml`.
- Parse and validate manifest fields: `name`, `protocol`, `entrypoint`, `commands`, `config_keys`.
- Enforce trust checks from SPEC: entrypoint under `plugins_dir`, no path traversal, entrypoint executable, reject world-writable plugin directories.
- Refuse to load plugins with unsupported `protocol`.

## Narrative

