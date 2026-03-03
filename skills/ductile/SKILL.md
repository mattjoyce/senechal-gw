---
name: ductile
description: >
  Operate, configure, and troubleshoot the ductile integration gateway CLI.
  Use this skill when the user wants to: run ductile commands, manage plugins or
  pipelines, configure the gateway, inspect jobs, trigger API calls, manage tokens/webhooks,
  validate or lock config, or understand ductile architecture.
  Triggers: any mention of "ductile", "config lock", "plugin list", "job inspect",
  "system start", pipeline DSL, or integration gateway tasks.
---

# Ductile CLI Skill

Ductile is a lightweight YAML-configured integration gateway for personal automation.
Design philosophy: polyglot plugins, event-driven pipelines, LLM-first CLI.

## Runtime Context

- **Binary**: `/home/matt/admin/ductile-test/ductile` (test) or `/home/matt/.local/bin/ductile` (local prod)
- **Test env**: `/home/matt/admin/ductile-test/`
- **Local prod config**: `~/.config/ductile/`
- **Run from**: `/home/matt/admin`
- **Pattern**: `ductile <noun> <action> [flags]`

## CLI Command Reference

### System
```bash
ductile system start                      # Start gateway (foreground)
ductile system status [--json]            # Health: PID, state DB, plugins
ductile system watch                      # Real-time TUI monitor
ductile system reset <plugin>             # Reset circuit breaker
ductile system skills [--config <dir>]    # Export LLM skill manifest (Markdown)
```

### Config
```bash
ductile config check [--json] [--strict]  # Validate syntax, policy, integrity
ductile config lock                       # Authorize state (update .checksums)
ductile config show [entity]              # Show resolved config or entity
ductile config get <path>                 # Dot-notation read (e.g. plugins.echo.enabled)
ductile config set <path>=<value>         # Modify value (with --dry-run to preview)
ductile config init                       # Initialize config directory
ductile config backup                     # Create backup archive
ductile config restore                    # Restore from backup
ductile config token create               # Interactively create scoped API token
ductile config token                      # Manage tokens
ductile config scope                      # Manage token scopes
ductile config plugin                     # Manage plugin configuration
ductile config route                      # Manage event routes
ductile config webhook                    # Manage webhooks
```

### Job
```bash
ductile job inspect <job_id> [-v]         # Lineage, baggage, workspace artifacts
```

### Plugin
```bash
ductile plugin list                       # Discover loaded plugins
ductile plugin run <name>                 # Manual execution
```

## Universal Flags
| Flag | Purpose |
|------|---------|
| `--json` | Machine-readable output (all read commands) |
| `-v, --verbose` | Internal logic, path resolution, baggage merges |
| `--dry-run` | Preview mutations without committing |
| `--config <dir>` | Override config directory |

## Entity Addressing
Use `<type>:<name>` syntax with `config show/get/set`:
```bash
ductile config show plugin:withings
ductile config show pipeline:video-wisdom
ductile config set plugin:withings.enabled=false
ductile config show plugin:*          # list all plugins
```

## LLM Capability Discovery
Ductile is designed for LLM operation. Use `system skills` to get the current live manifest:
```bash
ductile system skills --config ~/.config/ductile/
```
Or set `DUCTILE_CONFIG_DIR` and run `ductile system skills`.

This outputs a Markdown skill manifest listing all plugin commands with endpoints, schemas, and semantic anchors (`mutates_state`, `idempotent`, `retry_safe`), plus all configured pipelines.

## Common Workflows

### After any config change:
```bash
ductile config check          # Validate first
ductile config lock           # Authorize new state
```

### Trigger a pipeline via API:
```bash
curl -X POST http://localhost:8081/pipeline/<name> \
  -H "Authorization: Bearer $DUCTILE_LOCAL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"payload": {"key": "value"}}'
```

### Trigger a plugin directly (bypasses routing):
```bash
curl -X POST http://localhost:8081/plugin/<name>/poll \
  -H "Authorization: Bearer $DUCTILE_LOCAL_TOKEN" \
  -d '{"payload": {}}'
```

### Inspect a failed job:
```bash
ductile job inspect <job_id> -v --json
```

### Check gateway health:
```bash
curl http://localhost:8081/healthz
```

## Architecture Summary

- **Governance Hybrid**: Control plane (SQLite `event_context` baggage) + Data plane (filesystem workspaces)
- **Spawn-per-command**: Each plugin invocation is a fresh process (polyglot: bash, python, go, any executable)
- **At-least-once**: Jobs survive crashes and are recovered on restart
- **Immutable audit**: `origin_*` baggage keys can never be overwritten by plugins

## Config Integrity (Tiered)

| Tier | Files | On Mismatch |
|------|-------|-------------|
| High Security | `tokens.yaml`, `webhooks.yaml`, `scopes/*.json` | Hard fail (refuses to start) |
| Operational | `config.yaml`, `plugins/*.yaml`, `pipelines/*.yaml` | Warn & continue |

Always run `config check` then `config lock` after authorizing changes.

## Job Statuses
`queued` → `running` → `succeeded` / `failed` / `timed_out` / `dead`

## Reference Files
- **Config structure & grafting**: See [references/config.md](references/config.md)
- **REST API endpoints**: See [references/api.md](references/api.md)
- **Pipeline DSL & orchestration**: See [references/pipelines.md](references/pipelines.md)
