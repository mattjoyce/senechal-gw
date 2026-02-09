# Senechal Gateway — Specification

**Version:** 1.0
**Date:** 2026-02-08
**Author:** Matt Joyce
**Sources:** RFC-001, RFC-002, RFC-002-Decisions

This is the unified, buildable specification for Senechal Gateway. It supersedes all prior RFCs and review documents.

---

## 1. Overview

### 1.1 Problem

Senechal currently exists as a FastAPI monolith handling health data ETL, LLM processing, and various integrations. Adding new connectors means modifying the core application. Existing integration servers (n8n, Huginn, Node-RED) are too heavy for a personal service.

### 1.2 Solution

A lightweight, YAML-configured, modular integration gateway. A compiled Go core orchestrates polyglot plugins via a subprocess protocol. Simple enough to understand in an afternoon. Extensible enough to grow with new connectors.

### 1.3 Scope

This is a **personal integration server** processing roughly 50 jobs per day. Design decisions are calibrated to that scale. The system runs unattended and must behave predictably under crash, retry, and timeout conditions.

---

## 2. Architecture

```
┌─────────────────────────────────────────────┐
│                 senechal-gw                  │
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
│  │  result → route events → update        │  │
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

### 2.1 Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Core language | Go | Single binary, easy deployment, natural subprocess spawning |
| Plugin coupling | Subprocess (JSON over stdin/stdout) | Language-agnostic, fault-isolated, drop-in plugins |
| Scheduling | Heartbeat with fuzzy intervals | Human-friendly, avoids thundering herd |
| Execution | Serial FIFO dispatch | Simple, predictable, no concurrency bugs |
| Routing | Config-declared, fan-out, exact match | Plugins stay dumb, core controls flow |
| State | SQLite | Proven, zero-ops, single JSON blob per plugin |
| Delivery | At-least-once | Plugins own idempotency; core never drops work |
| Plugin lifecycle | Spawn-per-command | Eliminates daemon management, memory leaks, zombie processes |

---

## 3. Work Queue

The central abstraction. All producers submit to a single queue.

### 3.1 Producers

| Producer | Trigger |
|----------|---------|
| Scheduler | Heartbeat tick finds a plugin is due |
| Webhook receiver | Inbound HTTP event |
| Router | Plugin output matches a routing rule |
| CLI | Manual `senechal-gw run <plugin>` |

### 3.2 Job Model

```
{
  id:              UUID
  plugin:          string
  command:         string (poll | handle)
  payload:         JSON
  status:          queued | running | succeeded | failed | timed_out | dead
  attempt:         int (starts at 1)
  max_attempts:    int (default 4)
  submitted_by:    string (scheduler | webhook | route | cli)
  dedupe_key:      string (optional)
  created_at:      timestamp
  started_at:      timestamp (null until running)
  completed_at:    timestamp (null until terminal)
  next_retry_at:   timestamp (null unless awaiting retry)
  last_error:      text (null unless failed)
  parent_job_id:   UUID (null unless created by routing)
  source_event_id: UUID (null unless created by routing)
}
```

No `priority` field. Jobs are strictly FIFO.

### 3.3 Job State Machine

```
queued → running → succeeded
                 → failed → queued (retry)
                          → dead (max retries exceeded)
         → timed_out → queued (retry)
                     → dead (max retries exceeded)
```

### 3.4 Delivery Guarantee

**At-least-once.** A job may run more than once (after crash, timeout, or retry). It will never be silently dropped.

- Plugins MUST be idempotent, or use `state` to track what they've already processed.
- The core provides an opt-in `dedupe_key` field. If a job is enqueued with a `dedupe_key` matching a job that succeeded within `dedupe_ttl`, it is not enqueued. The drop is logged at `INFO` with the `dedupe_key` and existing job ID.
- `dedupe_ttl` is configurable (default 24h).

### 3.5 Dispatch

**Serial, single lane.** One job at a time, FIFO. No priority lanes. No concurrency.

Revisit condition: daily job count exceeds 500, or median queue wait time exceeds 30 seconds — with data to back it up.

### 3.6 Deduplication

When a producer enqueues a job with a `dedupe_key`:

1. Query for a `succeeded` job with the same `dedupe_key` completed within `dedupe_ttl`.
2. If found: do not enqueue. Log at `INFO`: dedupe_key, existing job ID.
3. If not found: enqueue normally.

---

## 4. Scheduler

A single internal tick loop. Each tick, the scheduler checks which plugins are due based on their configured interval and enqueues `poll` jobs.

### 4.1 Fuzzy Intervals

```yaml
plugins:
  withings:
    schedule:
      every: 6h
      jitter: 30m
      preferred_window:
        start: "06:00"
        end: "22:00"
```

Supported intervals: `5m`, `15m`, `30m`, `hourly`, `2h`, `6h`, `daily`, `weekly`, `monthly`.

No crontab syntax.

### 4.2 Jitter

Jitter computed **per scheduled run**, not per tick:

```
next_run = last_successful_run + interval + random(-jitter/2, +jitter/2)
```

Fixed for that scheduled run — no re-randomization per tick (prevents schedule wander). `preferred_window` is a hard constraint: if `next_run` falls outside the window, it snaps to the start of the next valid window.

### 4.3 Poll Guard

The scheduler **must not enqueue** a new `poll` job if there is already a `queued` or `running` `poll` job for that plugin. Configurable per-plugin (default 1):

```yaml
plugins:
  withings:
    max_outstanding_polls: 1
```

---

## 5. Plugin System

### 5.1 Lifecycle: Spawn-Per-Command

One process per job. No long-lived plugin processes.

1. Fork the plugin entrypoint.
2. Write JSON request to stdin.
3. Close stdin.
4. Read stdout until EOF or timeout.
5. Capture stderr.
6. Collect exit code.
7. Kill the process if it hasn't exited.

Process spawn overhead is ~5ms on Linux — irrelevant when the shortest interval is 5 minutes.

**Persistent connections (WebSockets, long-polling) are out of scope.** If needed, run as a separate service that pushes events into Senechal via the webhook endpoint. No streaming plugin mode — not now, not ever for this core.

### 5.2 Commands

| Command | Purpose | When |
|---------|---------|------|
| `poll` | Fetch data from external source | Scheduled by heartbeat |
| `handle` | Process an inbound event | Routed from another plugin or webhook |
| `health` | Diagnostic check | On-demand via `senechal-gw status` |
| `init` | One-time setup | On first discovery or config change |

- `init` is not retried on failure — plugin is marked unhealthy.
- `health` is not called on a schedule — it's a diagnostic tool for the operator.

### 5.3 Plugin Directory Structure

```
plugins/
├── withings/
│   ├── manifest.yaml
│   └── run.py
├── google-calendar/
│   ├── manifest.yaml
│   └── run.py
├── notify/
│   ├── manifest.yaml
│   └── run.sh
└── lib/            # shared helpers (e.g. OAuth utilities)
```

### 5.4 Manifest

```yaml
name: withings
version: 1.0.0
protocol: 1
entrypoint: run.py
description: "Fetch health data from Withings API"
commands: [poll, handle, health]
config_keys:
  required: [client_id, client_secret]
  optional: [access_token]
```

- `protocol` — must match a version the core supports. Mismatch → plugin not loaded.
- `entrypoint` — mandatory. Core constructs execution path as `<plugins_dir>/<plugin_name>/<entrypoint>`.
- `config_keys.required` — validated at load time. Missing keys → plugin not loaded, error logged.

### 5.5 Trust & Execution

- Plugins MUST live under `plugins_dir`. Symlinks resolved, must resolve within `plugins_dir`.
- `..` in `entrypoint` is rejected (path traversal prevention).
- Entrypoint MUST be executable (`chmod +x`). Shebang line handles interpreter selection.
- World-writable plugin directories are refused at load time.
- Plugins run as the same OS user as the core. Use systemd `User=senechal` to limit blast radius.

### 5.6 Timeouts

**Defaults:**

| Command | Timeout |
|---------|---------|
| `poll` | 60s |
| `handle` | 120s |
| `health` | 10s |
| `init` | 30s |

**Enforcement:**

1. Core starts a deadline timer when spawning the process.
2. On timeout: `SIGTERM` to the process group.
3. 5-second grace period.
4. `SIGKILL` if still alive.
5. Job status → `timed_out`, follows retry policy.

**Configurable per-plugin:**

```yaml
plugins:
  slow-plugin:
    timeouts:
      poll: 300s
      handle: 300s
```

**Resource caps:**
- Max stdout: 10 MB. Truncated with logged warning.
- Max stderr: 1 MB. Truncated.

### 5.7 Retry & Backoff

- Default: 4 attempts total (1 original + 3 retries).
- Backoff: `base * 2^(attempt-1) + random(0, base)` where `base = 30s`.
- Retry delays: ~30s, ~1m, ~2m (then dead).

**Non-retryable conditions:**
- Plugin exits with code `78` (EX_CONFIG from sysexits.h) — configuration error.
- Plugin response includes `"retry": false`.
- All other failures are retried.

**Configurable per-plugin:**

```yaml
plugins:
  withings:
    retry:
      max_attempts: 5
      backoff_base: 60s
```

### 5.8 Circuit Breaker

Configurable consecutive failure threshold per `(plugin, command)` pair. Applies to **scheduler-originated poll jobs only** — webhook-triggered `handle` jobs are not blocked by poll failures.

- Default threshold: 3 consecutive failures.
- Default reset: 30 minutes.
- Manual reset: `senechal-gw reset <plugin>`.

```yaml
plugins:
  withings:
    circuit_breaker:
      threshold: 3
      reset_after: 30m
```

### 5.9 State Model

**Config is static. State is dynamic.**

- `config` — from `config.yaml`, interpolated with env vars, read-only. Contains credentials, endpoints — things the operator sets.
- `state` — single JSON blob per plugin in SQLite. Plugins read it, return `state_updates`, core applies shallow merge (top-level keys replaced, not deep-merged).

```sql
plugin_state (
  plugin_name TEXT PRIMARY KEY,
  state       JSON NOT NULL DEFAULT '{}',
  updated_at  TIMESTAMP
)
```

**Size limit:** 1 MB per plugin state blob. Exceeding this rejects the update and fails the job.

### 5.10 OAuth

Plugins manage their own OAuth token lifecycle. The core does not understand OAuth.

- `client_id`, `client_secret` → `config` (static, set by operator).
- `access_token`, `refresh_token`, `token_expiry` → `state` (dynamic, managed by plugin).
- Plugin checks expiry on each invocation, refreshes if needed, returns new tokens via `state_updates`.
- Shared OAuth helpers can live in `plugins/lib/`.

---

## 6. Protocol (v1)

### 6.1 Request Envelope (core → plugin)

Single JSON object written to plugin's stdin:

```json
{
  "protocol": 1,
  "job_id": "uuid",
  "command": "poll | handle | health | init",
  "config": {},
  "state": {},
  "event": {},
  "deadline_at": "ISO8601"
}
```

- `event` — present only for `handle`.
- `state` — the plugin's full state blob.
- `deadline_at` — informational. Plugins MAY use it to abandon long-running work early. The core enforces the real deadline externally.

### 6.2 Response Envelope (plugin → core)

Single JSON object written to plugin's stdout:

```json
{
  "status": "ok | error",
  "error": "human-readable message (when status=error)",
  "retry": true,
  "events": [],
  "state_updates": {},
  "logs": []
}
```

- `retry` — defaults to `true` if omitted. Set `false` for permanent failures.
- `events` — array of event envelopes (see 6.3).
- `state_updates` — shallow-merged into plugin state.
- `logs` — array of `{"level": "info|warn|error", "message": "..."}`. Optional. Stored with the job record.

### 6.3 Event Envelope

Every event emitted by a plugin in the `events` array:

```json
{
  "type": "new_health_data",
  "payload": {},
  "dedupe_key": "withings:weight:2026-02-08"
}
```

- `type` — matches `event_type` in routing config. Exact string match.
- `payload` — arbitrary JSON, passed to downstream plugin's `handle` command.
- `dedupe_key` — optional. Downstream job inherits this as its `dedupe_key`.

The core injects when creating downstream jobs:
- `source` — plugin name.
- `timestamp` — ISO8601.
- `event_id` — UUID assigned by the core.

### 6.4 Framing

Single JSON object on stdout. Not JSON Lines, not length-prefixed. One request, one response, process exits.

### 6.5 Protocol Mismatch

If the request `protocol` field doesn't match what the plugin expects, the plugin SHOULD exit with code `78` (EX_CONFIG) and a clear error on stderr. The core refuses to load plugins whose manifest declares a `protocol` version it doesn't support.

---

## 7. Routing

Plugin chaining is declared in config, not by plugins. Plugins produce typed events; the config says where they go.

### 7.1 Config

```yaml
routes:
  - from: withings
    event_type: new_health_data
    to: health-analyzer

  - from: health-analyzer
    event_type: alert
    to: notify
```

### 7.2 Semantics

- **Fan-out:** A single event can match multiple routes. All matching routes produce a job.
- **No match:** Logged at DEBUG, dropped. Not an error.
- **Matching:** Exact string match on `event_type` only. No wildcards, no regexes, no glob patterns.
- **No conditional filters.** No `payload.severity == "high"`. If you need conditional logic, put it in the receiving plugin — it can inspect the payload and no-op.

### 7.3 Traceability

When the router creates a downstream job from an event:
- `parent_job_id` is set to the producing job's ID.
- `source_event_id` is set to the core-assigned `event_id`.

---

## 8. API Endpoints

The HTTP API allows external systems (LLMs, scripts, other services) to programmatically trigger plugin execution and retrieve job results.

### 8.1 Configuration

```yaml
api:
  enabled: true
  listen: "localhost:8080"
  auth:
    api_key: ${API_KEY}  # Bearer token for authentication
```

### 8.2 POST /trigger/{plugin}/{command}

Enqueues a job for the specified plugin and command. Returns immediately with a job ID (fire-and-forget, asynchronous execution).

**Request:**
- URL params: `{plugin}` - plugin name, `{command}` - command name (e.g., "poll")
- Header: `Authorization: Bearer <api_key>`
- Body: Optional JSON payload (passed to plugin as `config` override or additional parameters)

**Response (202 Accepted):**
```json
{
  "job_id": "uuid-v4",
  "status": "queued",
  "plugin": "plugin_name",
  "command": "command_name"
}
```

**Error Responses:**
- `401 Unauthorized` - Missing or invalid API key
- `400 Bad Request` - Plugin not found or command not supported
- `500 Internal Server Error` - Failed to enqueue job

**Example:**
```bash
curl -X POST http://localhost:8080/trigger/echo/poll \
  -H "Authorization: Bearer my-api-key-123" \
  -H "Content-Type: application/json" \
  -d '{}'
```

### 8.3 GET /job/{job_id}

Retrieves the status and results of a previously triggered job.

**Request:**
- URL param: `{job_id}` - UUID returned from POST /trigger
- Header: `Authorization: Bearer <api_key>`

**Response (200 OK - queued):**
```json
{
  "job_id": "uuid-v4",
  "status": "queued",
  "plugin": "plugin_name",
  "command": "command_name",
  "created_at": "2026-02-09T10:00:00Z"
}
```

**Response (200 OK - running):**
```json
{
  "job_id": "uuid-v4",
  "status": "running",
  "plugin": "plugin_name",
  "command": "command_name",
  "started_at": "2026-02-09T10:00:05Z"
}
```

**Response (200 OK - completed):**
```json
{
  "job_id": "uuid-v4",
  "status": "completed",
  "plugin": "plugin_name",
  "command": "command_name",
  "result": {
    "status": "ok",
    "state_updates": {"last_run": "2026-02-09T10:00:10Z"},
    "logs": [{"level": "info", "message": "Plugin executed successfully"}]
  },
  "started_at": "2026-02-09T10:00:05Z",
  "completed_at": "2026-02-09T10:00:10Z"
}
```

**Error Responses:**
- `401 Unauthorized` - Missing or invalid API key
- `404 Not Found` - Job ID not found

### 8.4 Authentication

Simple bearer token authentication. Single API key configured via `api.auth.api_key` in config.yaml.

- Key should be stored in environment variable and interpolated: `${API_KEY}`
- All API requests must include `Authorization: Bearer <api_key>` header
- Invalid or missing key returns `401 Unauthorized`
- No key rotation mechanism in MVP (manual config update + reload)

### 8.5 Use Cases

- **LLM Tool Calling:** LLM agents can curl /trigger to execute actions (e.g., "check my calendar", "sync Withings data")
- **External Automation:** Scripts, cron jobs, or other services can trigger plugins programmatically
- **Result Polling:** External systems can poll /job/{id} to wait for async plugin execution completion
- **Manual Testing:** Developers can trigger plugins via curl without waiting for scheduler

---

## 9. Webhooks

### 9.1 Listener

```yaml
webhooks:
  listen: 127.0.0.1:8081
  endpoints:
    - path: /hook/github
      plugin: github-handler
      secret: ${GITHUB_WEBHOOK_SECRET}
      signature_header: X-Hub-Signature-256
      max_body_size: 1MB
```

### 9.2 Security

HMAC-SHA256 signature verification is **mandatory** for all webhook endpoints.

1. Read raw request body (up to `max_body_size`, default 1 MB).
2. Compute `HMAC-SHA256(secret, raw_body)`.
3. Compare against the signature header (configurable name per endpoint).
4. Reject with `403` if invalid. No error details in response.
5. Reject with `413` if body exceeds `max_body_size`.

No replay protection in V1. No rate limiting in V1 (proxy responsibility if fronted by reverse proxy).

### 9.3 Health Endpoint

`/healthz` on the webhook listener port:

```json
{
  "status": "ok",
  "uptime_seconds": 3600,
  "queue_depth": 2,
  "plugins_loaded": 5,
  "plugins_circuit_open": 0
}
```

No authentication. Localhost only. Useful for systemd watchdog and operator checks.

---

## 10. Operations

### 10.1 Single-Instance Lock

PID file with `flock(LOCK_EX | LOCK_NB)`:

1. Create/open `<state_dir>/senechal-gw.lock`.
2. Acquire `flock`. Fail → log error, exit 1.
3. Write current PID.
4. Lock held for process lifetime. Kernel releases on crash/exit.

### 10.2 Crash Recovery

On startup:

1. Open the SQLite database.
2. Acquire the exclusive lock.
3. Find all jobs with `status = running` — orphans from a prior crash.
4. For each orphan: increment `attempt`, set `status = queued` if under `max_attempts`, else `status = dead`.
5. Log each recovered job at WARN level.
6. Resume normal dispatch.

### 10.3 Config Reload

`senechal-gw reload` sends `SIGHUP` to the running process (found via PID file).

On SIGHUP:

1. Parse new config. If invalid → log error, keep old config.
2. In-flight jobs continue with existing config snapshot.
3. Scheduler updates intervals/jitter for all plugins.
4. Router updates routing rules.
5. Plugin config changes take effect on next dispatch.
6. Newly added plugins discovered → `init` runs.
7. Removed/disabled plugins → queued jobs cancelled (status → `dead`), no new jobs enqueued.

### 10.4 Logging

**Core logs:** JSON to stdout.

Fields: `timestamp`, `level`, `component`, `plugin` (when relevant), `job_id` (when relevant), `message`.

**Plugin stderr:** Captured. Always. Stored in `job_log` (capped at 64 KB). Logged at WARN to core log stream.

**Plugin stdout:** Reserved exclusively for protocol response. Stored verbatim on completion in `job_log.result` (JSON). Non-JSON on stdout is a protocol error — job fails, stderr + stdout captured for debugging.

**Redaction:** Not in V1. Don't log secrets. Fix the plugin, don't bandage the core.

### 10.5 Job Log Retention

Pruned on every scheduler tick:

```sql
DELETE FROM job_log WHERE completed_at < datetime('now', '-30 days')
```

Default 30 days. Configurable via `service.job_log_retention`.

### 10.6 CLI

```
senechal-gw start              # run the service (foreground)
senechal-gw run <plugin>       # manually run a plugin once
senechal-gw status             # show plugin states, queue depth, last runs
senechal-gw reload             # reload config without restart
senechal-gw reset <plugin>     # reset circuit breaker for a plugin
senechal-gw plugins            # list discovered plugins
senechal-gw logs [plugin]      # tail structured logs
senechal-gw queue              # show pending/active jobs
```

---

## 11. Database Schema

### 10.1 Tables

```sql
-- Job queue (active and historical)
job_queue (
  id              TEXT PRIMARY KEY,   -- UUID
  plugin          TEXT NOT NULL,
  command         TEXT NOT NULL,       -- poll | handle
  payload         JSON,
  status          TEXT NOT NULL,       -- queued | running | succeeded | failed | timed_out | dead
  attempt         INTEGER NOT NULL DEFAULT 1,
  max_attempts    INTEGER NOT NULL DEFAULT 4,
  submitted_by    TEXT NOT NULL,       -- scheduler | webhook | route | cli
  dedupe_key      TEXT,
  created_at      TEXT NOT NULL,       -- ISO8601
  started_at      TEXT,
  completed_at    TEXT,
  next_retry_at   TEXT,
  last_error      TEXT,
  parent_job_id   TEXT,                -- FK to job_queue.id
  source_event_id TEXT                 -- UUID assigned by core
);

-- Plugin state (one row per plugin)
plugin_state (
  plugin_name TEXT PRIMARY KEY,
  state       JSON NOT NULL DEFAULT '{}',
  updated_at  TEXT
);

-- Job log (completed jobs for audit/debugging)
job_log (
  id              TEXT PRIMARY KEY,
  plugin          TEXT NOT NULL,
  command         TEXT NOT NULL,
  status          TEXT NOT NULL,
  result          TEXT,                -- protocol response JSON
  attempt         INTEGER NOT NULL,
  submitted_by    TEXT NOT NULL,
  created_at      TEXT NOT NULL,
  completed_at    TEXT NOT NULL,
  last_error      TEXT,
  stderr          TEXT,                -- capped at 64 KB
  parent_job_id   TEXT,
  source_event_id TEXT
);
```

---

## 12. Configuration Reference

Complete `config.yaml` with all supported fields and defaults:

```yaml
service:
  name: senechal-gw
  tick_interval: 60s           # scheduler tick frequency
  log_level: info              # debug | info | warn | error
  log_format: json
  dedupe_ttl: 24h              # deduplication window
  job_log_retention: 30d       # prune completed jobs older than this

api:
  enabled: false
  listen: 127.0.0.1:8080
  auth:
    api_key: ${SENECHAL_API_KEY}

state:
  path: ./data/state.db

plugins_dir: ./plugins

plugins:
  withings:
    enabled: true
    schedule:
      every: 6h
      jitter: 30m
      preferred_window:
        start: "06:00"
        end: "22:00"
    config:
      client_id: ${WITHINGS_CLIENT_ID}
      client_secret: ${WITHINGS_CLIENT_SECRET}
    retry:
      max_attempts: 4          # default: 4 (1 original + 3 retries)
      backoff_base: 30s        # default: 30s
    timeouts:
      poll: 60s                # default: 60s
      handle: 120s             # default: 120s
    circuit_breaker:
      threshold: 3             # default: 3 consecutive failures
      reset_after: 30m         # default: 30m
    max_outstanding_polls: 1   # default: 1

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
      signature_header: X-Hub-Signature-256
      max_body_size: 1MB

routes:
  - from: withings
    event_type: new_health_data
    to: health-analyzer

  - from: health-analyzer
    event_type: alert
    to: notify
```

Environment variable interpolation via `${VAR}` syntax. Secrets never stored in the config file itself.

---

## 13. Deployment

### 13.1 Systemd Unit

```ini
[Unit]
Description=Senechal Gateway
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/senechal-gw start --config /etc/senechal-gw/config.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
User=senechal
Group=senechal

[Install]
WantedBy=multi-user.target
```

### 13.2 Development

Run `senechal-gw start` directly. No systemd required.

---

## 14. Project Layout

```
senechal-gw/
├── cmd/
│   └── senechal-gw/
│       └── main.go
├── internal/
│   ├── config/
│   ├── queue/
│   ├── scheduler/
│   ├── dispatch/
│   ├── plugin/
│   ├── state/
│   ├── api/
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

## 15. Implementation Phases

| Phase | Sprint | Scope | Status |
|-------|--------|-------|--------|
| 1. Skeleton | 0 | Go scaffold, CLI, config loader, SQLite state, plugin discovery | ✅ Complete |
| 2. Core Loop | 1 | Work queue, heartbeat scheduler with fuzzy intervals, dispatch loop, plugin protocol, crash recovery | ✅ Complete |
| 3. API Triggers | 2 | HTTP server with chi router, POST /trigger and GET /job endpoints, Bearer token auth, job result storage | 🔄 In Progress |
| 4. Routing | 3 | Config-declared event routing, downstream enqueuing, event_id traceability | Planned |
| 5. Webhooks | 3 | HTTP listener, HMAC verification, /healthz, route inbound webhooks to plugins | Planned |
| 6. Reliability Controls | 4 | Circuit breaker, retry with exponential backoff, deduplication enforcement | Planned |
| 7. CLI & Ops | 5 | Status/run/reload/reset/plugins/queue/logs commands, systemd unit | Planned |
| 8. First Plugins | 6 | Port Withings & Garmin from existing Senechal, notify plugin | Planned |

**Note:** Phase 3 (API Triggers) was prioritized before Routing and Webhooks to enable LLM-driven automation via curl-based triggers. This allows external systems to programmatically enqueue jobs and retrieve results immediately, accelerating the path to production use cases.

---

## 16. Deferred Decisions

| Topic | Rationale |
|-------|-----------|
| Two-tier stderr/stdout caps (capture vs persistence) | Current spec is workable. Clarify post-V1 if storage becomes a concern. |
| `protocol` field in response envelope | Can be added in protocol v2 without breaking v1 plugins. |
| Replay protection for webhooks | Provider-specific. Add per-plugin if a provider requires it. |
| Rate limiting on webhook listener | Proxy responsibility. Core doesn't duplicate concerns it can't own. |
| Secret redaction in logs | Operator responsibility. Fix the plugin, don't bandage the core. |
| Streaming / long-lived plugin mode | Out of scope permanently. If it needs to stream, it's not a plugin. |
| Priority queues / multi-lane dispatch | Revisit only if daily jobs exceed 500 or median wait exceeds 30s. |
| Router query language / payload filters | Put conditional logic in the receiving plugin. |
