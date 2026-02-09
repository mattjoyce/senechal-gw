---
id: 19
status: done
priority: Normal
blocked_by: []
tags: [sprint-1, mvp, logging]
---

# Structured JSON Logging

Emit machine-readable logs so the service can run unattended and be debugged from stdout/journald.

## Acceptance Criteria
- Core logs JSON with fields: `timestamp`, `level`, `component`, `plugin` (when relevant), `job_id` (when relevant), `message`.
- Captured plugin stderr is logged at WARN and stored (with caps) where applicable.

## Narrative
- 2026-02-08: Implemented structured JSON logging in `internal/log/logger.go` using Go's `log/slog` with JSON handler. Provides `Setup()` for global configuration, helper functions `WithComponent()`, `WithPlugin()`, `WithJob()` for context attachment, and wraps `slog.Logger` for consistent structured fields. All logs emit JSON with timestamp, level, component, and contextual fields (plugin, job_id) when available. Tests verify context helper behavior. Merged via PR #2. (by @gemini)
