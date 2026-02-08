---
id: 12
status: done
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
- **2026-02-08: Complete.** Implemented `internal/plugin` package with:
  - `manifest.go`: Manifest and Plugin types, SupportsCommand helper
  - `discovery.go`: Registry, Discover scanner, trust validation (SPEC §5.5)
  - `discovery_test.go`: Comprehensive tests for discovery, validation, trust checks (all passing)
  - Security: Enforces symlink resolution, path traversal prevention, executable check, world-writable rejection
  - Protocol v1 enforcement: Rejects unsupported protocol versions
  - Graceful degradation: Invalid plugins logged but don't fail startup

