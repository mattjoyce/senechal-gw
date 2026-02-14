# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ductile is a lightweight, YAML-configured integration gateway written in Go. It orchestrates polyglot plugins via a subprocess protocol (JSON over stdin/stdout). The system is designed for personal-scale automation (~50 jobs/day) with emphasis on simplicity, reliability, and predictable behavior under crash/retry/timeout conditions.

**Core Philosophy:** Simple enough to understand in an afternoon. Extensible enough to grow with new connectors. Plugins stay dumb; the core controls flow.

## Architecture

### Central Abstractions

1. **Work Queue (SQLite-backed)** - All producers submit to a single FIFO queue:
   - Producers: Scheduler (heartbeat ticks), Webhook receiver, Router (plugin output), CLI
   - Serial dispatch (one job at a time, no concurrency)
   - At-least-once delivery guarantee (plugins must be idempotent)

2. **Plugin Lifecycle: Spawn-Per-Command** - No long-lived plugin processes:
   - Fork entrypoint → write JSON to stdin → read JSON from stdout → kill process
   - Protocol v1: single request/response, process exits
   - Timeouts enforced: SIGTERM → 5s grace → SIGKILL

3. **State Model:**
   - `config` - Static, from config.yaml, interpolated with env vars (credentials, endpoints)
   - `state` - Dynamic, single JSON blob per plugin in SQLite, shallow-merged on updates
   - Plugins manage their own OAuth lifecycle via state (access_token, refresh_token in state)

### Project Structure (from SPEC.md §13)

```
ductile/
├── cmd/ductile/           # CLI entrypoint
│   └── main.go
├── internal/                   # Core components (not importable externally)
│   ├── config/                # YAML parser, ${ENV} interpolation
│   ├── queue/                 # SQLite work queue (enqueue/dequeue/state machine)
│   ├── scheduler/             # Heartbeat tick, fuzzy intervals, poll guard
│   ├── dispatch/              # Plugin spawn, stdin/stdout, timeout enforcement
│   ├── plugin/                # Discovery, manifest validation, registry
│   ├── state/                 # SQLite plugin_state table, shallow merge
│   ├── webhook/               # HTTP listener, HMAC verification, /healthz
│   └── router/                # Config-declared event routing, fan-out
├── plugins/                    # Drop-in plugins (any language with shebang)
│   └── example/
│       ├── manifest.yaml      # name, protocol, entrypoint, commands, config_keys
│       └── run.py             # Executable with protocol v1 JSON I/O
└── config.yaml                # Service config, plugin schedules, routes, webhooks
```

**Internal packages are organized by concern, not by layer.** Each package owns a distinct responsibility.

## Protocol v1 (SPEC.md §6)

**Request envelope (core → plugin stdin):**
```json
{
  "protocol": 1,
  "job_id": "uuid",
  "command": "poll | handle | health | init",
  "config": {},
  "state": {},
  "event": {},           // only for handle
  "deadline_at": "ISO8601"
}
```

**Response envelope (plugin stdout → core):**
```json
{
  "status": "ok | error",
  "error": "message",    // when status=error
  "retry": true,         // default true; false = non-retryable
  "events": [],          // array of {type, payload, dedupe_key}
  "state_updates": {},   // shallow-merged into plugin state
  "logs": []             // [{level, message}] - stored with job
}
```

**Framing:** Single JSON object, not JSON Lines. One request, one response, process exits.

## Key Design Constraints

1. **Serial Dispatch** - No concurrency, strictly FIFO. Revisit only if daily jobs > 500 or median wait > 30s.
2. **No Streaming** - No WebSockets, long-polling, or persistent plugin connections. If it needs to stream, it's not a plugin (run as separate service pushing to webhook endpoint).
3. **Exact Match Routing** - No wildcards, regexes, or payload filters. Conditional logic belongs in receiving plugins.
4. **Spawn-Per-Command** - No daemon management. Process spawn overhead (~5ms) is irrelevant at this scale.
5. **HMAC Mandatory** - All webhook endpoints require HMAC-SHA256 signature verification (SPEC.md §8.2).

## Configuration Reference (SPEC.md §11)

Environment variable interpolation via `${VAR}` syntax. Secrets never in config file.

**Critical fields:**
- `service.tick_interval` - Scheduler heartbeat (default 60s)
- `service.dedupe_ttl` - Deduplication window (default 24h)
- `plugins.<name>.schedule.every` - Supported: 5m, 15m, 30m, hourly, 2h, 6h, daily, weekly, monthly (no crontab)
- `plugins.<name>.schedule.jitter` - Per-run randomization to avoid thundering herd
- `plugins.<name>.circuit_breaker.threshold` - Consecutive failures before blocking scheduler (default 3)
- `routes` - Array of `{from, event_type, to}` for plugin chaining (fan-out supported)

## Implementation Status

**Current phase:** Pre-MVP (planning complete, no code yet)

**MVP Scope (MVP.md):** Prove core loop works end-to-end:
- Config loader, SQLite state, PID lock, scheduler tick, dispatch loop, protocol v1 codec
- Echo plugin for validation (bash script, reads stdin, writes stdout)
- Crash recovery (orphaned `running` jobs → `dead` on restart, no retry in MVP)
- Structured JSON logging

**NOT in MVP:** Routing, webhooks, circuit breaker, deduplication, retry/backoff, config reload, health endpoint

**Implementation phases (SPEC.md §14):**
1. Skeleton (Go scaffold, CLI, config, SQLite, plugin discovery)
2. Core Loop (queue, scheduler, dispatch, protocol)
3. Routing (event fan-out, traceability)
4. Webhooks (HTTP listener, HMAC, /healthz)
5. CLI & Ops (status/reload/reset, systemd unit)
6. First Plugins (port Withings/Garmin from existing Ductile)

## Multi-Agent Coordination

**See COORDINATION.md for complete guide** when working with multiple agents in parallel.

**Quick Reference:**
- **Branching:** Individual feature branches per component (`feature/config-loader`, `feature/state-queue`)
- **Commits:** `<component>: <action> <what>` (never attribute Claude/AI)
- **Work Assignment:** 3-agent split defined in COORDINATION.md with dependency graph
- **Merge Order:** Skeleton → Foundation (parallel) → Integration (collaborative)

## Development Commands

**Note:** Project is pre-code. Commands below are from SPEC for when implementation begins.

```bash
# Build
go build -o ductile ./cmd/ductile

# Run tests
go test ./...

# Run service (foreground)
./ductile start --config config.yaml

# Planned CLI commands (SPEC.md §9.6)
./ductile run <plugin>     # manually trigger plugin once
./ductile status           # plugin states, queue depth, last runs
./ductile system monitor   # real-time TUI dashboard
./ductile reload           # SIGHUP config reload
./ductile reset <plugin>   # reset circuit breaker
./ductile plugins          # list discovered plugins
./ductile queue            # show pending/active jobs
```

## Testing Strategy

**MVP test plugin (MVP.md):** `plugins/echo/run.sh` - Reads stdin, returns JSON with timestamp in `state_updates.last_run`.

**Validation checklist:**
- Plugin spawned on schedule with jitter
- Protocol v1 JSON valid on stdin
- Response parsed, state persisted to SQLite
- Stderr captured and logged
- Timeout kills hung plugin (test with `sleep 999`)
- Crash recovery: kill -9 process, restart, orphaned job logged

## Kanban Workflow

Tasks tracked in `kanban/*.md` with YAML frontmatter:
- `id`, `status` (todo/doing/done), `priority`, `blocked_by`, `tags`
- Sprint-1 cards focus on MVP core loop (see `kanban/sprint-1-mvp-core-loop.md`)
- Epic cards: `sprint-1-mvp-core-loop`, `sprint-2-routing`, `sprint-3-webhooks-healthz`, `sprint-4-reliability-controls`

## Critical References

- **SPEC.md** - Single source of truth, supersedes all RFCs
- **MVP.md** - Minimal subset to prove architecture
- **RFC/** - Multi-LLM design reviews (historical context, not authoritative)
- **Go idioms:** Simple, explicit error handling, composition over inheritance, no panic/recover in plugins

## Deferred Decisions (SPEC.md §15)

Do NOT implement these without explicit requirement:
- Protocol v2 changes (response envelope `protocol` field)
- Replay protection for webhooks (provider-specific)
- Rate limiting on webhook listener (proxy responsibility)
- Secret redaction in logs (fix plugins, don't bandage core)
- Priority queues / multi-lane dispatch (only if jobs > 500/day)
- Router query language / payload filters (put logic in receiving plugin)
