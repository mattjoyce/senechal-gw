# CLAUDE.md — Ductile

Guidance for AI agents working in this repository.

## What is Ductile

Ductile is a lightweight, YAML-configured integration gateway written in Go. It orchestrates polyglot plugins via a subprocess protocol (JSON over stdin/stdout). Designed for personal-scale automation (~50–500 jobs/day) with emphasis on simplicity, reliability, and predictable behaviour under crash, retry, and timeout conditions.

**Core philosophy:** Plugins stay dumb; the core controls flow. Simple enough to understand in an afternoon. Extensible enough to grow with new connectors.

**Current status:** Production (v1.0.0-rc.1). Running on Thinkpad (`~/.local/bin/ductile`) and Unraid.

## Architecture

### Central Abstractions

1. **Work Queue (SQLite-backed)** — All producers submit to a single FIFO queue. Producers: Scheduler (heartbeat ticks), Webhook receiver, Router (plugin output), CLI/API. Serial dispatch by default (`max_workers: 1`); configurable per-plugin with `parallelism`.

2. **Plugin Lifecycle: Spawn-Per-Command** — No long-lived plugin processes:
   - Fork entrypoint → write JSON to stdin → read JSON from stdout → kill process
   - Protocol v2: request includes `context` (baggage) and `workspace_dir`
   - Timeouts enforced: SIGTERM → 5s grace → SIGKILL

3. **State Model:**
   - `config` — Static, from config files, interpolated with env vars
   - `state` — Dynamic, single JSON blob per plugin in SQLite, shallow-merged on updates
   - `workspace` — Per-job directory on disk for file artifacts; inherited across pipeline steps

4. **Pipeline Engine** — YAML DSL for event-driven orchestration. Steps can `uses` a plugin, `call` another pipeline, `split` for parallel fan-out, gate with `if` conditions, and remap payload with `with`.

### Project Structure

```
ductile/
├── cmd/ductile/           # CLI entrypoint
├── internal/              # Core packages (not importable externally)
│   ├── api/               # REST API server
│   ├── auth/              # Token auth, scopes
│   ├── config/            # YAML parser, ${ENV} interpolation, integrity
│   ├── dispatch/          # Plugin spawn, preflight, workspace, pipeline routing
│   ├── doctor/            # Startup preflight checks
│   ├── e2e/               # End-to-end test harness
│   ├── events/            # Event hub
│   ├── inspect/           # Job/baggage inspection
│   ├── lock/              # PID lock
│   ├── log/               # Structured logging
│   ├── plugin/            # Discovery, manifest validation, registry
│   ├── protocol/          # v2 codec (request/response envelopes)
│   ├── queue/             # SQLite work queue, state machine
│   ├── router/            # Config-declared event routing, fan-out
│   ├── scheduler/         # Heartbeat tick, interval/cron, poll guard
│   ├── scheduleexpr/      # Schedule expression parser
│   ├── state/             # SQLite plugin_state table, shallow merge
│   ├── storage/           # SQLite helpers
│   ├── tui/               # Terminal UI (system watch)
│   ├── webhook/           # HTTP listener, HMAC verification
│   └── workspace/         # Workspace lifecycle (create, clone, shard)
├── plugins/               # Bundled reference plugins
│   ├── echo/              # Minimal bash reference plugin
│   ├── fabric/            # Fabric pattern runner
│   ├── fetch/             # HTTP fetch
│   ├── file_handler/      # File write/copy
│   ├── file_watch/        # File system watcher
│   ├── folder_watch/      # Folder watcher
│   ├── sys_exec/          # Shell command executor
│   └── ...
├── skills/ductile/        # Claude Code skill (install to ~/.claude/skills/ductile/)
├── docs/                  # Canonical documentation
│   ├── ARCHITECTURE.md    # Single source of truth — supersedes all RFCs
│   ├── API_REFERENCE.md   # REST API endpoints and schemas
│   ├── CONFIG_REFERENCE.md # Configuration spec
│   ├── PIPELINES.md       # Pipeline DSL reference
│   ├── PLUGIN_DEVELOPMENT.md # Plugin authoring guide
│   └── OPERATOR_GUIDE.md  # Day-to-day operations
├── config.yaml            # Example/template service config
└── go.mod
```

## Protocol v2

**Request envelope (core → plugin stdin):**
```json
{
  "protocol": 2,
  "job_id": "uuid",
  "command": "poll | handle | health | init",
  "config": {},
  "state": {},
  "context": {},
  "workspace_dir": "/path/to/workspace",
  "event": {},
  "deadline_at": "ISO8601"
}
```

**Response envelope (plugin stdout → core):**
```json
{
  "status": "ok | error",
  "result": "short human-readable summary",
  "error": "message",
  "retry": true,
  "events": [{"type": "...", "payload": {}}],
  "state_updates": {},
  "logs": [{"level": "info", "message": "..."}]
}
```

`result` is required when `status: ok`. Events trigger downstream pipeline routing.

## Configuration

Ductile uses a tiered config directory (default `~/.config/ductile/`). Only `config.yaml` is implicitly loaded; everything else is pulled in via `include:`.

**Typical layout:**
```
~/.config/ductile/
├── config.yaml          # Service settings, plugin_roots, include list
├── api.yaml             # API server config and tokens
├── tokens.yaml          # Scoped API tokens (High Security — integrity enforced)
├── webhooks.yaml        # Webhook endpoints and secrets (High Security)
├── plugins/             # Per-group plugin configs (loaded via include: plugins/)
│   ├── discord.yaml
│   ├── web.yaml
│   └── ...
├── pipelines.yaml       # Pipeline DSL
├── .checksums           # BLAKE3 integrity manifest (updated by ductile config lock)
└── .env                 # Local env vars
```

**`config.yaml` structure:**
```yaml
service:
  strict_mode: true        # Hard fail on any integrity/config check failure

state:
  path: ~/.config/ductile/ductile.db

plugin_roots:
  - ~/Projects/ductile/plugins
  - ~/Projects/ductile-plugins

webhooks:
  listen: "0.0.0.0:8091"

environment_vars:
  include:
    - .env
    - ~/.config/secrets/anthropic/.env

include:
  - api.yaml
  - tokens.yaml
  - plugins/
  - pipelines.yaml
  - webhooks.yaml
```

**Plugin aliasing** — reuse a plugin implementation with a different identity and config:
```yaml
plugins:
  github_interest_notify:
    uses: discord_notify      # Inherits discord_notify's implementation
    enabled: true
    config:
      webhook_url: "${DISCORD_WEBHOOK_URL}"
      message_template: "⭐ {sender.login} starred {repository.full_name}"
```

**Schedules** — interval or cron:
```yaml
plugins:
  discord_notify:
    schedules:
      - cron: "0 9 * * *"
        command: poll
        timezone: "Australia/Sydney"
      # OR interval:
      - every: 30m
        jitter: 2m
```

## Pipeline DSL

Pipelines are event-driven workflows defined in `pipelines.yaml`.

```yaml
pipelines:
  - name: youtube-wisdom
    on: youtube.url.detected        # Trigger event type (exact match)
    steps:
      - id: transcript
        uses: youtube_transcript
      - id: summarize
        uses: fabric
        with:
          pattern: "{payload.pattern}"   # Payload remap before plugin spawn
      - id: write
        uses: file_handler
        if:                              # Conditional execution
          path: payload.status
          op: eq
          value: ok

  - name: notify-on-failure
    on-hook: job.completed             # Lifecycle hook (system event, not plugin event)
    steps:
      - uses: discord_notify
        if:
          path: payload.status
          op: neq
          value: succeeded
```

Step types: `uses` (invoke plugin), `call` (invoke pipeline), `split` (parallel fan-out).

`if` operators: `exists`, `eq`, `neq`, `in`, `gt`, `gte`, `lt`, `lte`, `contains`, `startswith`, `endswith`, `regex`. Composable with `all`, `any`, `not`.

Execution modes: `async` (default, fire-and-forget) or `synchronous` (API blocks for result, use with `timeout:`).

## Key Design Constraints

1. **Serial Dispatch by default** — `max_workers: 1`. Increase only if daily jobs > 500 or median wait > 30s.
2. **No Streaming** — No WebSockets or persistent plugin connections.
3. **Exact Match Routing** — No wildcards. Conditional logic belongs in plugins or `if` predicates.
4. **Spawn-Per-Command** — No daemon management. ~5ms spawn overhead is fine at this scale.
5. **HMAC Mandatory** — All webhook endpoints require HMAC-SHA256.
6. **Integrity on High-Security files** — `tokens.yaml`, `webhooks.yaml`, `scopes/*.json` must pass BLAKE3 check or system refuses to start. Always run `ductile config lock` after editing these files.

## Development Commands

```bash
# Build
go build -o ductile ./cmd/ductile

# Test
go test ./...

# Run (foreground)
./ductile system start --config ~/.config/ductile/config.yaml

# After editing any config files
ductile config lock
ductile config check
```

## Workflow

- **Branching:** `<component>/card<id>-<short-description>`
- **Commits:** `<component>: <action> <what>` — never attribute Claude/AI
- **Rule:** One card → one branch → one PR → merge → next card
- **Issue tracking:** `bd` (beads) CLI — see `AGENTS.md`

## Ductile Skill

A Claude Code skill for operating ductile is in `skills/ductile/`. Install it:

```bash
cp -r skills/ductile/ ~/.claude/skills/ductile/
```

## Critical References

- `docs/ARCHITECTURE.md` — Single source of truth, supersedes all RFCs
- `docs/API_REFERENCE.md` — REST API
- `docs/CONFIG_REFERENCE.md` — Config spec
- `docs/PIPELINES.md` — Pipeline DSL
- `docs/PLUGIN_DEVELOPMENT.md` — Plugin authoring
- `docs/OPERATOR_GUIDE.md` — Day-to-day ops
