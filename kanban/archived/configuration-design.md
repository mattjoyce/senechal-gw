---
id: 4
status: done
priority: Normal
blocked_by: []
tags: [configuration, yaml]
---

# Configuration Design

YAML schema for service config, connectors, and routing.

## Acceptance Criteria
- Top-level config schema is defined (service, connectors, routes, schedules)
- Connector-specific config sections are designed
- Environment variable interpolation is supported
- Config validation approach is chosen
- Example config file is drafted

## Narrative
- 2026-02-08: Created during initial project discussion. YAML-based config is a core design goal — keep it human-readable and lightweight. (by @assistant)
- 2026-02-08: Fully specified in RFC-002 Decisions. Config schema covers: service-level settings (dedupe_ttl, job_log_retention), webhook endpoints (path, plugin, secret, signature_header, max_body_size), per-plugin sections (retry, timeouts, circuit_breaker, max_outstanding_polls). Env var interpolation via ${VAR} syntax. Validation: required config_keys checked at plugin load time. Consolidated config reference in RFC-002-Decisions.md. (by @assistant)
