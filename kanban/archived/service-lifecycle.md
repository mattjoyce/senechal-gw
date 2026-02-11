---
id: 7
status: done
priority: Normal
blocked_by: []
tags: [operations, service]
---

# Service Lifecycle

Systemd integration, logging, health checks.

## Acceptance Criteria
- Systemd unit file template is defined
- Graceful startup and shutdown sequence is designed
- Health check endpoint/mechanism is specified
- Structured logging format is chosen
- Log rotation and retention approach is defined
- Reload-on-config-change behavior is decided

## Narrative
- 2026-02-08: Created during initial project discussion. User wants this to run as a service, similar to existing Senechal setup with systemd. (by @assistant)
- 2026-02-08: Fully specified in RFC-002 Decisions. Startup: acquire flock on PID file, recover orphaned jobs, resume dispatch. Shutdown: kernel releases lock. Reload: SIGHUP parses new config, in-flight jobs continue, scheduler/router update, new plugins init'd, removed plugins cancelled. Health: /healthz on webhook port (status, uptime, queue_depth, plugins_loaded, circuits_open). Logging: JSON to stdout (timestamp, level, component, plugin, job_id, message). Retention: job_log pruned to configurable TTL (default 30d). systemd User=senechal recommended. (by @assistant)
