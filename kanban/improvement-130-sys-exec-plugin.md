---
id: 130
status: doing
priority: Normal
blocked_by: []
tags: [improvement, plugin, sys-exec, shell, infrastructure]
---

# sys_exec Plugin — Generic Shell Command Executor

## Job Story

When I need Ductile to run a shell command as part of a pipeline, I want a generic `sys_exec` plugin that I can copy and configure with any command, so I can wire arbitrary system operations (builds, deploys, syncs) into event-driven workflows without writing a new plugin from scratch.

## Motivation

Ductile needs a way to run shell commands as pipeline steps. The immediate driver is triggering an Astro site build after new content is written (card #129), but the pattern is broadly useful:

- SSH remote commands (`ssh host "docker exec container npm run build"`)
- Local docker commands
- Scripts and build triggers

Rather than a dedicated Astro plugin, a generic `sys_exec` plugin provides the right level of abstraction: operators configure the command, the plugin just runs it.

## Design

### Plugin Type

Copy-and-configure: operators copy `plugins/sys_exec/` to a new directory, rename it, and set `command` in config. Each instance has a distinct plugin name and command.

### Manifest

```yaml
manifest_spec: ductile.plugin
manifest_version: 1
name: sys_exec
version: 0.1.0
protocol: 2
entrypoint: run.py
description: "Execute a configured shell command and emit result as an event"
commands:
  - name: handle
    type: write
    description: "Run the configured command. Payload fields available as environment variables."
  - name: health
    type: read
    description: "Verify command is configured and (optionally) dry-run check."
config_keys:
  required: [command]
  optional: [timeout_seconds, working_dir, env]
```

### Behaviour

- Runs `command` via subprocess shell
- Payload fields injected as env vars (`DUCTILE_PAYLOAD_*`)
- Captures stdout + stderr
- Non-zero exit → `status: error`, retryable based on exit code
- Emits `sys_exec.completed` event with `{exit_code, stdout, stderr, command}`

### Security Model

- **Command is config, not payload** — the command string comes from `config.yaml`, not the incoming event. Operators control what runs.
- **Config is signed** — Ductile config integrity is operator-controlled; no runtime command injection from events.
- **File permissions** — entrypoint must be executable; plugin directory ownership should be restricted.
- **No shell interpolation of payload fields** — payload fields are passed as environment variables only, never interpolated into the command string.
- **Allowlist env vars** — only `DUCTILE_PAYLOAD_*` and a safe subset of system env are passed through.

## Acceptance Criteria

- [x] `sys_exec` plugin implemented with `handle` and `health` commands
- [x] `command` is required config; plugin refuses to start without it
- [x] Payload fields available as `DUCTILE_PAYLOAD_{KEY}` env vars (uppercased)
- [x] stdout/stderr captured and returned in logs and event payload
- [x] Non-zero exit returns `status: error` with stderr as error message
- [x] `health` command validates `command` is set
- [x] No shell interpolation of payload fields into command string
- [x] Manifest includes `config_keys.required: [command]`
- [ ] Works as the Astro refresh step in card #129 pipeline

## Example Config (Astro Refresh)

```yaml
plugins:
  astro-refresh:
    enabled: true
    timeout: 120s
    max_attempts: 1
    config:
      command: "ssh unraid 'docker exec astro npm run build'"
```

## Narrative

- 2026-02-26: Created as a prerequisite for card #129 (Discord → YouTube → Astro RSS). Identified during planning as the right abstraction for running shell commands in pipelines rather than building a dedicated Astro plugin. Security model: command from signed config only, payload as env vars, no interpolation. (by @assistant)
- 2026-02-26: Moved to doing. Confirmed `plugins/sys_exec/` does not exist yet; starting with a design hardening pass focused on copy/rename ergonomics, command execution safety, env shaping, output bounds, and retry semantics before implementation. (by @assistant)
- 2026-02-26: Implemented `plugins/sys_exec` with copier-oriented comments in both `manifest.yaml` and `run.py`. Added protocol-v2 behavior for `handle` and `health`, required `config.command`, payload-to-env mapping (`DUCTILE_PAYLOAD_*`), bounded stdout/stderr capture, configurable retry-on-exit-codes, and completion event emission. Added e2e tests (`internal/e2e/sys_exec_plugin_test.go`) for success, non-zero exit, retry policy, and health validation. Full `go test ./...` passes. Remaining card item is end-to-end validation in the card #129 Astro pipeline wiring. (by @assistant)
