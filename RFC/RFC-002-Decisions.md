# RFC-002: Operational Semantics — Decisions

**Status:** Accepted
**Date:** 2026-02-08
**Author:** Matt Joyce
**Reviewed by:** Claude (Opus 4.6), OpenAI, Gemini
**Depends on:** RFC-001

---

## Final Decisions

These decisions incorporate RFC-002 as drafted plus accepted amendments from the consensus review. This is the binding specification for implementation.

---

### 1. Delivery Guarantee: At-Least-Once

Ductile guarantees **at-least-once delivery**. A job may run more than once. It will never be silently dropped.

- Plugins MUST be idempotent, or use `state` to track what they've already processed.
- The core provides a `dedupe_key` field on jobs. If a job is enqueued with a `dedupe_key` matching a job that succeeded within `dedupe_ttl`, it is **not enqueued** and the drop is logged at `INFO` with the `dedupe_key` and existing job ID.
- `dedupe_ttl` is configurable (default 24h):

```yaml
service:
  dedupe_ttl: 24h
```

- "Notify" plugins that send emails/messages MUST use state to track sent notification IDs.

---

### 2. Job State Machine

```
queued → running → succeeded
                 → failed → queued (retry)
                          → dead (max retries exceeded)
         → timed_out → queued (retry)
                     → dead (max retries exceeded)
```

**Job model:**

```
{
  id:              UUID
  plugin:          string
  command:         string (poll | handle)
  payload:         JSON
  status:          queued | running | succeeded | failed | timed_out | dead
  attempt:         int (starts at 1)
  max_attempts:    int (default 4: 1 original + 3 retries)
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

No `priority`. Jobs are FIFO.

---

### 3. Retry & Backoff

- Default: 4 attempts total (1 original + 3 retries).
- Backoff: `base * 2^(attempt-1) + random(0, base)` where `base = 30s`.
- Retry delays: ~30s, ~1m, ~2m (then dead).
- Configurable per-plugin:

```yaml
plugins:
  withings:
    retry:
      max_attempts: 5
      backoff_base: 60s
```

**Non-retryable conditions:**
- Plugin exits with code `78` (EX_CONFIG) — configuration error.
- Plugin response includes `"retry": false`.
- All other failures are retried.

**Circuit breaker:** Configurable consecutive failure threshold per `(plugin, command)` pair. Default: 3 consecutive failures. Applies to **scheduler-originated poll jobs only** — webhook-triggered `handle` jobs are not blocked by poll failures. Resets after 30 minutes, or manually via `ductile reset <plugin>`.

```yaml
plugins:
  withings:
    circuit_breaker:
      threshold: 3
      reset_after: 30m
```

---

### 4. Plugin Timeouts

**Defaults:**
- `poll`: 60s
- `handle`: 120s
- `health`: 10s
- `init`: 30s

**Configurable per-plugin:**

```yaml
plugins:
  slow-plugin:
    timeouts:
      poll: 300s
      handle: 300s
```

**Enforcement:**
1. Core starts a deadline timer when spawning the plugin process.
2. On timeout: `SIGTERM` to the process group.
3. 5-second grace period.
4. `SIGKILL` if still alive.
5. Job status → `timed_out`, follows retry policy.

**Resource caps:**
- Max stdout: 10 MB. Truncated with logged warning.
- Max stderr: 1 MB. Truncated.

---

### 5. Plugin Lifecycle: Spawn-Per-Command

One process per job. No long-lived plugin processes.

1. Fork the plugin entrypoint.
2. Write JSON request to stdin.
3. Close stdin.
4. Read stdout until EOF or timeout.
5. Capture stderr.
6. Collect exit code.
7. Kill the process if it hasn't exited.

- `init` runs once on first discovery or config change. Not retried on failure — plugin marked unhealthy.
- `health` is called by `ductile status`, not on a schedule.
- Persistent connections (WebSockets, long-polling) are **out of scope**. If needed, run as a separate service that pushes events into Ductile via the webhook endpoint.
- No streaming plugin mode. Not now, not ever for this core.

---

### 6. Crash Recovery

On startup:

1. Open the SQLite database.
2. Acquire exclusive advisory lock (Decision 12).
3. Find all jobs with `status = running` — these are orphans from a prior crash.
4. For each orphan: increment `attempt`, set `status = queued` if under `max_attempts`, else `status = dead`.
5. Log each recovered job at WARN level.
6. Resume normal dispatch.

---

### 7. Protocol Specification (v1)

**Request envelope (core → plugin, JSON on stdin):**

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

- `event` present only for `handle`.
- `state` is the plugin's full state blob.
- `deadline_at` is informational — plugins MAY use it to abandon work early.

**Response envelope (plugin → core, JSON on stdout):**

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

- `retry` defaults to `true` if omitted. Set `false` for permanent failures.
- `logs` — array of `{"level": "info|warn|error", "message": "..."}`. Optional. Stored with the job record.

**Framing:** Single JSON object on stdout. One request, one response, process exits.

**Protocol mismatch:** Plugin SHOULD exit with code `78` (EX_CONFIG). Core refuses to load plugins whose manifest declares an unsupported `protocol` version.

**Deferred:** Adding `protocol` field to the response envelope — revisit in protocol v2.

**Manifest:**

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

---

### 8. Event Envelope

Every event emitted by a plugin:

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

The core injects `source` (plugin name), `timestamp` (ISO8601), and `event_id` (UUID) when creating the downstream job.

---

### 9. Plugin State Model

- `config` — static, from `config.yaml`, read-only.
- `state` — single JSON blob per plugin in SQLite, shallow merge on `state_updates`.

```sql
plugin_state (
  plugin_name TEXT PRIMARY KEY,
  state       JSON NOT NULL DEFAULT '{}',
  updated_at  TIMESTAMP
)
```

**Size limit:** 1 MB per plugin state blob. Exceeding this rejects the update and fails the job.

---

### 10. OAuth: Plugin-Owned

Plugins manage their own OAuth token lifecycle. The core does not understand OAuth.

- `client_id`, `client_secret` → `config` (static).
- `access_token`, `refresh_token`, `token_expiry` → `state` (dynamic).
- Plugin checks expiry on each invocation, refreshes if needed, returns new tokens via `state_updates`.
- Shared OAuth helpers can live in `plugins/lib/`.

---

### 11. Webhook Security

HMAC-SHA256 signature verification is mandatory for all webhook endpoints.

1. Read raw request body (up to `max_body_size`, default 1 MB).
2. Compute `HMAC-SHA256(secret, raw_body)`.
3. Compare against configurable signature header.
4. Reject with `403` if invalid. No error details in response.
5. Reject with `413` if body exceeds `max_body_size`.

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

No replay protection in V1. No rate limiting in V1 (proxy responsibility).

---

### 12. Multi-Instance Lock

PID file with `flock(LOCK_EX | LOCK_NB)`.

1. Create/open `<state_dir>/ductile.lock`.
2. Acquire `flock`. Fail → log error, exit 1.
3. Write current PID.
4. Lock held for process lifetime. Kernel releases on crash/exit.

---

### 13. Config Reload

`ductile reload` sends `SIGHUP` to the running process.

On SIGHUP:
1. Parse new config. If invalid → log error, keep old config.
2. In-flight jobs continue with existing config snapshot.
3. Scheduler updates intervals/jitter.
4. Router updates routing rules.
5. Plugin config changes take effect on next dispatch.
6. New plugins discovered → `init` runs.
7. Removed/disabled plugins → queued jobs cancelled (status → `dead`), no new jobs enqueued.

---

### 14. Dispatch: Serial, Single Lane

One job at a time, FIFO. No priority lanes. No concurrency.

**Revisit condition:** Daily job count exceeds 500, or median queue wait time exceeds 30 seconds — and someone is actually complaining with data to back it up.

---

### 15. Routing Semantics

- **Fan-out:** A single event can match multiple routes. All matches fire.
- **No match:** Logged at DEBUG, dropped. Not an error.
- **Matching:** Exact string match only. No wildcards, no regexes.
- **No conditional filters in V1.** Put conditional logic in the receiving plugin.

---

### 16. Plugin Trust & Execution

- Plugins MUST live under `plugins_dir`. Symlinks resolved, must resolve within `plugins_dir`.
- `manifest.yaml` with explicit `entrypoint` field required.
- Execution path: `<plugins_dir>/<plugin_name>/<entrypoint>`. `..` in entrypoint rejected.
- Entrypoint MUST be executable (shebang handles interpreter).
- World-writable plugin directories refused at load time.
- Plugins run as same OS user as core. Use systemd `User=ductile`.

---

### 17. Jitter Behavior

Jitter computed **per scheduled run**:

```
next_run = last_successful_run + interval + random(-jitter/2, +jitter/2)
```

Fixed for that scheduled run — no re-randomization per tick. `preferred_window` is a hard constraint: `next_run` snaps to start of next valid window if outside.

---

### 18. Logging & stderr

- **Core logs:** JSON to stdout. Fields: `timestamp`, `level`, `component`, `plugin`, `job_id`, `message`.
- **Plugin stderr:** Captured, stored in `job_log` (capped at 64 KB), logged at WARN to core log stream.
- **Plugin stdout:** Reserved for protocol response. Non-JSON on stdout is a protocol error.
- **Redaction:** Not in V1. Don't log secrets. Fix the plugin, don't bandage the core.

---

### 19. Job Log Retention

```sql
DELETE FROM job_log WHERE completed_at < datetime('now', '-30 days')
```

Configurable:

```yaml
service:
  job_log_retention: 30d
```

---

### 20. Core Health Endpoint

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

No authentication (localhost only).

---

### 21. Scheduler Poll Guard

The scheduler **must not enqueue** a new `poll` job if there is already a `queued` or `running` `poll` job for that plugin. Configurable max outstanding polls per plugin (default 1):

```yaml
plugins:
  withings:
    max_outstanding_polls: 1
```

This prevents unbounded poll backlog under serial dispatch.

---

## Deferred Decisions

| Topic | Rationale |
|-------|-----------|
| Two-tier stderr/stdout caps (capture vs persistence) | Current spec is workable. Clarify post-V1 if storage becomes a concern. |
| `protocol` field in response envelope | Can be added in protocol v2 without breaking v1 plugins. |

---

## Config Reference (Consolidated)

```yaml
service:
  dedupe_ttl: 24h
  job_log_retention: 30d

webhooks:
  listen: 127.0.0.1:8081
  endpoints:
    - path: /hook/github
      plugin: github-handler
      secret: ${GITHUB_WEBHOOK_SECRET}
      signature_header: X-Hub-Signature-256
      max_body_size: 1MB

plugins:
  withings:
    retry:
      max_attempts: 5
      backoff_base: 60s
    timeouts:
      poll: 300s
      handle: 300s
    circuit_breaker:
      threshold: 3
      reset_after: 30m
    max_outstanding_polls: 1
```
