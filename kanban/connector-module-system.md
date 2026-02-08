---
id: 3
status: done
priority: High
blocked_by: []
tags: [architecture, connectors, modularity]
---

# Connector/Module System

Design pluggable connector architecture for Google, Withings, and other external services.

## Acceptance Criteria
- Connector interface/contract is defined
- Registration and discovery mechanism is designed
- Configuration per-connector via YAML is specified
- Lifecycle management (init, health check, teardown) is clear
- Auth credential handling per connector is addressed

## Narrative
- 2026-02-08: Created during initial project discussion. User's key requirement is modularity — easy to add new connectors for Google, Withings, etc. Existing Senechal has Garmin and Withings ETL patterns to draw from. (by @assistant)
- 2026-02-08: Fully specified in RFC-002 Decisions. Plugin contract: spawn-per-command, JSON stdin/stdout protocol (v1), manifest.yaml with entrypoint/protocol/config_keys. Discovery via plugins_dir. Lifecycle: init (once), poll, handle, health (on-demand). OAuth plugin-owned. State as single JSON blob per plugin, shallow merge. Configurable timeouts, retries, circuit breaker per plugin. (by @assistant)
