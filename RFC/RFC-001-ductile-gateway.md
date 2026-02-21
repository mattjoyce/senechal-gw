# RFC-001: Ductile

**Status:** Historical Draft (superseded by RFC-002-Decisions)
**Date:** 2026-02-08
**Author:** Matt Joyce

---

## Problem

Ductile currently exists as a FastAPI monolith handling health data ETL, LLM processing, and various integrations. It works, but adding new connectors means modifying the core application. Existing integration servers (n8n, Huginn, Node-RED) are too heavy for a personal service.

## Proposal

A **lightweight, YAML-configured, modular integration gateway** — a compiled Go core that orchestrates polyglot plugins via a subprocess protocol. It should be simple enough to understand in an afternoon, but extensible enough to grow with new connectors.

---

## Architecture Overview

```
┌─────────────────────────────────────────────┐
│                 ductile                  │
│              (Go binary, ~1 process)         │
│                                              │
│  ┌───────────┐  ┌──────────┐  ┌───────────┐ │
│  │ Scheduler  │  │ Webhook  │  │   CLI     │ │
│  │ (heartbeat)│  │ Receiver │  │ Commands  │ │
│  └─────┬──────┘  └────┬─────┘  └─────┬─────┘ │
│        │              │              │        │
│        ▼              ▼              ▼        │
│  ┌────────────────────────────────────────┐  │
│  │            WORK QUEUE                  │  │
│  │  (in-memory, SQLite-backed for         │  │
│  │   persistence/crash recovery)          │  │
│  └──────────────────┬─────────────────────┘  │
│                     │                         │
│                     ▼                         │
│  ┌────────────────────────────────────────┐  │
│  │         DISPATCH LOOP (serial)         │  │
│  │  pull job → spawn plugin → collect     │  │
│  │  result → enqueue downstream → update  │  │
│  │  state → repeat                        │  │
│  └──────────────────┬─────────────────────┘  │
│                     │                         │
│  ┌──────────┐  ┌────┴─────┐  ┌────────────┐ │
│  │  Config  │  │  State   │  │  Plugin    │ │
│  │  Loader  │  │  Store   │  │  Registry  │ │
│  │  (YAML)  │  │ (SQLite) │  │            │ │
│  └──────────┘  └──────────┘  └────────────┘ │
└─────────────────────┬───────────────────────┘
                      │ stdin/stdout JSON protocol
        ┌─────────────┼─────────────┐
        ▼             ▼             ▼
   ┌─────────┐  ┌──────────┐  ┌─────────┐
   │withings/ │  │ google/  │  │ notify/ │
   │ run.py   │  │ run.py   │  │ run.sh  │
   └─────────┘  └──────────┘  └─────────┘
```

### Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Core language | Go | Single binary, easy deployment, natural subprocess spawning |
| Plugin coupling | Subprocess (JSON over stdin/stdout) | Language-agnostic, fault-isolated, drop-in plugins |
| Scheduling | Heartbeat with fuzzy intervals | Human-friendly, avoids thundering herd, adds natural variance |
| Execution | Serial dispatch | Simple, predictable, no concurrency issues. Fine for personal use |
| Routing | Config-declared | Plugins stay dumb, core controls flow, easy to reconfigure |
| State | SQLite | Proven, zero-ops, already used in Ductile |

---

## Core Components

### 1. Work Queue

The central abstraction. **Everything that produces work submits to the queue.** This unifies all trigger types under a single model.

**Producers:**
- **Scheduler** — heartbeat tick finds a plugin is due
- **Webhook receiver** — inbound HTTP event
- **Plugin output** — a plugin's result enqueues downstream work (declared in config)
- **CLI** — manual `ductile run <plugin>` enqueues a job

**Job structure:**
```
{id, plugin, command, payload, priority, submitted_by, created_at}
```

**Execution:** Serial — one job at a time.

**Persistence:** Queue state backed by SQLite so in-flight/pending jobs survive a crash or restart.

### 2. Scheduler (Heartbeat + Fuzzy Intervals)

A single internal tick loop (e.g. every 60s). Each tick, the scheduler checks which plugins are due based on their configured interval and enqueues jobs for them.

**Fuzzy scheduling** uses human-friendly intervals with jitter:

```yaml
plugins:
  withings:
    schedule:
      every: daily       # daily, hourly, weekly, monthly, 15m, 6h, etc.
      jitter: 2h         # random offset within this window
      preferred_window:   # optional: only run between these hours
        start: "06:00"
        end: "22:00"
```

No crontab syntax. Supported intervals: `5m`, `15m`, `30m`, `hourly`, `2h`, `6h`, `daily`, `weekly`, `monthly`.

Jitter: each cycle, the core picks a random offset within the jitter window. "Daily with 2h jitter" means once per day, sometime within a 2-hour window around the nominal time.

### 3. Plugin Protocol (JSON over stdin/stdout)

Plugins are **executables** in a known directory. The core spawns them as subprocesses and communicates via JSON lines over stdin/stdout.

**Commands the core sends:**

```json
{"command": "poll", "config": {...}, "state": {...}}

{"command": "handle", "event": {"type": "...", "payload": {...}}, "config": {...}, "state": {...}}

{"command": "health", "config": {...}}

{"command": "init", "config": {...}}
```

**Plugin response:**

```json
{
  "status": "ok",
  "events": [...],
  "state_updates": {...},
  "logs": [...]
}
```

**Plugin directory structure:**
```
plugins/
├── withings/
│   ├── manifest.yaml
│   └── run.py
├── google-calendar/
│   ├── manifest.yaml
│   └── run.py
└── notify/
    ├── manifest.yaml
    └── run.sh
```

**manifest.yaml:**
```yaml
name: withings
version: 1.0.0
description: "Fetch health data from Withings API"
commands: [poll, handle, health]
config_keys:
  - client_id
  - client_secret
  - access_token
```

### 4. Config-Declared Routing

Plugin chaining is declared in the main config, not by plugins themselves. Plugins stay dumb — they produce typed events, the config says where those events go.

```yaml
routes:
  - from: withings
    event_type: new_health_data
    to: health-analyzer

  - from: health-analyzer
    event_type: alert
    to: notify
```

When the dispatch loop collects a plugin's output events, it checks the routing table and enqueues downstream jobs accordingly.

### 5. Configuration (config.yaml)

```yaml
service:
  name: ductile
  tick_interval: 60s
  log_level: info
  log_format: json

state:
  path: ./data/state.db

plugins_dir: ./plugins

plugins:
  withings:
    enabled: true
    schedule:
      every: 6h
      jitter: 30m
    config:
      client_id: ${WITHINGS_CLIENT_ID}
      client_secret: ${WITHINGS_CLIENT_SECRET}

  google-calendar:
    enabled: true
    schedule:
      every: 15m
      jitter: 3m
    config:
      credentials_file: ${GOOGLE_CREDS_PATH}

webhooks:
  listen: 127.0.0.1:8081
  endpoints:
    - path: /hook/github
      plugin: github-handler
      secret: ${GITHUB_WEBHOOK_SECRET}

routes:
  - from: withings
    event_type: new_health_data
    to: health-analyzer

  - from: health-analyzer
    event_type: alert
    to: notify
```

Environment variable interpolation via `${VAR}` syntax. Secrets never in the config file itself.

### 6. State Store (SQLite)

Simple key-value per plugin, plus the job queue table.

**Tables:**
- `plugin_state` — `(plugin_name, key, value, updated_at)`
- `job_queue` — `(id, plugin, command, payload, priority, status, submitted_by, created_at, started_at, completed_at)`
- `job_log` — completed jobs for audit/debugging

Plugins receive their state slice with each invocation and return state updates. They never touch SQLite directly.

### 7. CLI

```
ductile start              # run the service (foreground)
ductile run <plugin>       # manually run a plugin once
ductile status             # show plugin states, queue depth, last runs
ductile reload             # reload config without restart
ductile plugins            # list discovered plugins
ductile logs [plugin]      # tail structured logs
ductile queue              # show pending/active jobs
```

### 8. Service Deployment

**Systemd unit (Linux production):**
```ini
[Unit]
Description=Ductile
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ductile start --config /etc/ductile/config.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
User=ductile
Group=ductile

[Install]
WantedBy=multi-user.target
```

**Development (any OS):** Just run `ductile start` directly.

---

## Project Layout

```
ductile/
├── cmd/
│   └── ductile/
│       └── main.go
├── internal/
│   ├── config/
│   ├── queue/
│   ├── scheduler/
│   ├── dispatch/
│   ├── plugin/
│   ├── state/
│   ├── webhook/
│   └── router/
├── plugins/
│   └── example/
│       ├── manifest.yaml
│       └── run.py
├── config.yaml
├── go.mod
├── go.sum
└── Makefile
```

---

## Implementation Phases

| Phase | Scope |
|-------|-------|
| 1. Skeleton | Go scaffold, CLI, config loader, SQLite state, plugin discovery |
| 2. Core Loop | Work queue, heartbeat scheduler with fuzzy intervals, dispatch loop, plugin protocol |
| 3. Routing | Config-declared event routing, downstream enqueuing, state updates |
| 4. Webhooks | HTTP listener, route inbound webhooks to plugins |
| 5. CLI & Ops | Status/run/reload/plugins/queue/logs commands, systemd unit, structured logging |
| 6. First Plugins | Port Withings & Garmin from existing Ductile, notify plugin |

---

## Open Questions

- Plugin timeout handling — what happens when a plugin hangs?
- Plugin stderr — capture as logs or discard?
- Config reload semantics — what happens to in-flight jobs when config changes?
- Multi-instance safety — should SQLite locking prevent two instances running simultaneously?
- Plugin versioning — how to handle manifest version mismatches?
- OAuth token refresh — who manages token lifecycle, the core or the plugin?

---

## Feedback Requested

This RFC is seeking critique on:
1. Is the work queue as central abstraction the right model?
2. Is subprocess (JSON over stdin/stdout) the right plugin boundary?
3. Is Go the right choice for the core, given polyglot plugins?
4. What's missing from this design?
5. What's over-engineered for a personal integration server?
