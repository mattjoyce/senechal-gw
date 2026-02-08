---
id: 19
status: todo
priority: Normal
blocked_by: [9]
tags: [sprint-1, mvp, logging]
---

# Structured JSON Logging

Emit machine-readable logs so the service can run unattended and be debugged from stdout/journald.

## Acceptance Criteria
- Core logs JSON with fields: `timestamp`, `level`, `component`, `plugin` (when relevant), `job_id` (when relevant), `message`.
- Captured plugin stderr is logged at WARN and stored (with caps) where applicable.

## Narrative

