# RFC-002: Operational Semantics

**Status:** Historical Draft (superseded by RFC-002-Decisions)
**Date:** 2026-02-08
**Author:** Matt Joyce
**Depends on:** RFC-001

---

## Purpose

RFC-001 established the architecture: Go core, work queue, subprocess plugins, config-declared routing, SQLite state. Three independent reviews (Gemini, OpenAI, Claude) converged on the same verdict: the architecture is right, but the operational semantics are underspecified.

This RFC closes every open question from RFC-001 and makes binding decisions on the behaviors that determine whether Ductile runs reliably unattended.

---

## Decisions

### 1. Delivery Guarantee: At-Least-Once

Ductile guarantees **at-least-once delivery**. A job may run more than once (after crash, timeout, or retry). It will never be silently dropped.

**Implications:**
- Plugins MUST be idempotent, or use `state` to track what they've already processed.
- The core provides a `dedupe_key` field on jobs. If a job is enqueued with a `dedupe_key` matching a job that succeeded within the last 24 hours, it is silently dropped. This is opt-in — producers set the key, the core enforces it.
- "Notify" plugins that send emails/messages MUST use state to track sent notification IDs. The core will not deduplicate on their behalf beyond `dedupe_key`.

**Rationale:** Exactly-once is a distributed systems fantasy for a subprocess-based system. At-most-once loses work. At-least-once with idempotent plugins is the proven sweet spot.

---

### 2. Job State Machine

Every job has a status. Transitions are enforced by the core:

```
queued → running → succeeded
                 → failed → queued (retry)
                          → dead (max retries exceeded)
         → timed_out → queued (retry)
                     → dead (max retries exceeded)
```

**Job model (expanded from RFC-001):**

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
}
```

**What was removed from RFC-001:** `priority`. Jobs are FIFO. There is no priority system. If you want something to run sooner, don't put slow things ahead of it. This is a personal server, not a job scheduler.

---

### 3. Retry & Backoff

**Policy:**
- Default: 4 attempts total (1 original + 3 retries).
- Backoff: exponential with jitter — `base * 2^(attempt-1) + random(0, base)` where `base = 30s`.
- Retry delays: ~30s, ~1m, ~2m (then dead).
- Configurable per-plugin in `config.yaml`:

```yaml
plugins:
  withings:
    retry:
      max_attempts: 5
      backoff_base: 60s
```

**Non-retryable conditions:**
- Plugin exits with code `78` (EX_CONFIG from sysexits.h) — configuration error, retrying won't help.
- Plugin response includes `"retry": false`.
- All other failures are retried.

**Circuit breaker:** If a plugin produces 5 consecutive failures (across any jobs), the scheduler stops enqueuing new jobs for that plugin. The circuit resets after 30 minutes, or manually via `ductile reset <plugin>`. Existing queued jobs for the plugin still execute (they may succeed and reset the circuit).

---

### 4. Plugin Timeouts

**Non-negotiable.** A hung plugin cannot stall the gateway.

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
2. On timeout: send `SIGTERM` to the process group.
3. 5-second grace period.
4. `SIGKILL` if still alive.
5. Job status → `timed_out`, follows retry policy.

**Resource caps:**
- Max stdout: 10 MB. Truncated with a logged warning beyond this.
- Max stderr: 1 MB. Truncated.
- These are generous for JSON. If you're hitting them, you're doing something wrong.

---

### 5. Plugin Lifecycle: Spawn-Per-Command

Plugins are **spawn-per-command**. One process per job. No long-lived plugin processes.

The core:
1. Forks the plugin entrypoint.
2. Writes the JSON request to stdin.
3. Closes stdin.
4. Reads stdout until EOF or timeout.
5. Captures stderr.
6. Collects exit code.
7. Kills the process if it hasn't exited.

**`init` runs once:** On first discovery (plugin directory appears with a valid manifest) or when the plugin's config changes. It is not retried on failure — the plugin is marked unhealthy and the operator is expected to fix the config and `reload`. `init` is for one-time setup like validating credentials, not for ongoing work.

**`health` is called by `ductile status`**, not on a schedule. It's a diagnostic tool for the operator, not a liveness probe.

**Rationale:** Long-lived processes mean managing heartbeats, reconnection, memory leaks, and zombie detection. Spawn-per-command eliminates all of that. Process spawn overhead is ~5ms on Linux — irrelevant when your shortest interval is 5 minutes.

---

### 6. Crash Recovery

On startup, the core:

1. Opens the SQLite database.
2. Acquires an exclusive advisory lock (see Decision 12).
3. Finds all jobs with `status = running`. These are orphans from a prior crash.
4. For each orphan: increment `attempt`, set `status = queued` if under `max_attempts`, else `status = dead`.
5. Log each recovered job at WARN level.
6. Resume normal dispatch.

No "stale timeout" is needed. If the process is alive, it holds the lock. If the lock is free, everything `running` is orphaned.

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

- `event` is present only for `handle`.
- `state` is the plugin's full state blob (see Decision 9).
- `deadline_at` is informational — plugins MAY use it to abandon long-running work early. The core enforces the real deadline externally.

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
- `events` — see Decision 8.
- `state_updates` — merged into plugin state (see Decision 9).
- `logs` — array of `{"level": "info|warn|error", "message": "..."}`. Optional. Stored with the job record.

**Framing:** Single JSON object on stdout. Not JSON Lines, not length-prefixed. One request, one response, process exits. This is spawn-per-command — there is no streaming.

**Protocol mismatch:** If the request `protocol` field is absent or doesn't match what the plugin expects, the plugin SHOULD exit with code `78` (EX_CONFIG) and a clear error on stderr. The core refuses to load a plugin whose manifest declares a `protocol` version it doesn't support.

**Manifest addition:**

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

Changes from RFC-001: added `protocol`, `entrypoint` (mandatory), and split `config_keys` into `required`/`optional`. The core validates that all `required` keys are present in config at load time. Missing required keys → plugin not loaded, error logged.

---

### 8. Event Envelope

Every event emitted by a plugin MUST have this shape:

```json
{
  "type": "new_health_data",
  "payload": {},
  "dedupe_key": "withings:weight:2026-02-08"
}
```

- `type` — matches `event_type` in routing config. String. No namespacing scheme — just be descriptive.
- `payload` — arbitrary JSON. Passed to the downstream plugin's `handle` command.
- `dedupe_key` — optional. If set, the downstream job inherits this as its `dedupe_key`.

The core injects `source` (plugin name) and `timestamp` (ISO8601) when creating the downstream job. Plugins don't set these.

---

### 9. Plugin State Model

**Config is static. State is dynamic.** Full stop.

- `config` comes from `config.yaml`, interpolated with env vars, and is read-only. It contains credentials, client IDs, endpoints — things the operator sets.
- `state` is a single JSON blob per plugin, stored in SQLite. Plugins read it, return `state_updates`, and the core merges updates (shallow merge — top-level keys replaced, not deep-merged).

**Schema:**

```sql
plugin_state (
  plugin_name TEXT PRIMARY KEY,
  state       JSON NOT NULL DEFAULT '{}',
  updated_at  TIMESTAMP
)
```

Not key-value. One row per plugin, one JSON blob. Simpler to reason about, simpler to back up, and plugins get their full state in one shot.

**Size limit:** 1 MB per plugin state blob. If a plugin's state exceeds this, the update is rejected and the job fails. If you need more, you're using state wrong — write to a file and store the path.

---

### 10. OAuth: Plugin-Owned

Plugins manage their own OAuth token lifecycle. The core does not understand OAuth.

- `client_id` and `client_secret` live in `config` (static, set by operator).
- `access_token`, `refresh_token`, `token_expiry` live in `state` (dynamic, managed by plugin).
- The plugin checks token expiry on each invocation, refreshes if needed, and returns the new tokens via `state_updates`.

**Rationale:** OAuth flows are provider-specific (some need PKCE, some don't, callback URLs vary, scopes differ). Putting OAuth in the core means the core must understand every provider. That's the opposite of "plugins stay dumb, core stays generic." The plugin is the right place — it already knows the provider's API.

A shared OAuth helper library (Python, shell) can live in `plugins/lib/` for reuse. That's a plugin-ecosystem concern, not a core concern.

---

### 11. Webhook Security

**HMAC-SHA256 signature verification is mandatory for all webhook endpoints.**

Behavior:
1. The webhook handler reads the raw request body (up to `max_body_size`, default 1 MB).
2. Computes `HMAC-SHA256(secret, raw_body)`.
3. Compares against the signature header. The header name is configurable per endpoint (GitHub uses `X-Hub-Signature-256`, others vary).
4. Rejects with `403` if invalid. No error details in the response.
5. Rejects with `413` if body exceeds `max_body_size`.

**Config:**

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

**No replay protection in V1.** Timestamp checking is provider-specific and most providers (GitHub, Stripe) don't mandate it. The HMAC signature is sufficient for a personal server. If a specific provider requires timestamp validation, the plugin can check it in `handle`.

**Rate limiting:** Not in V1. The webhook listener binds to localhost. If fronted by a reverse proxy, rate limiting belongs in the proxy. The core doesn't duplicate concerns it can't own.

---

### 12. Multi-Instance Lock

**PID file lock.** Simple, proven, zero-dependency.

On startup:
1. Attempt to create/open `<state_dir>/ductile.lock`.
2. Acquire `flock(LOCK_EX | LOCK_NB)` on the file.
3. Write the current PID.
4. If lock acquisition fails → log error, exit with code 1.
5. Lock is held for the lifetime of the process. Kernel releases it on crash/exit.

No SQLite-based locking. No heartbeat files. `flock` handles crash recovery correctly — the kernel cleans up.

---

### 13. Config Reload

**`ductile reload` sends SIGHUP to the running process** (found via the PID file from Decision 12).

On SIGHUP:
1. Parse the new config file. If invalid → log error, keep old config, done.
2. In-flight jobs continue with their existing config snapshot. They are not interrupted.
3. The scheduler updates intervals/jitter for all plugins based on new config.
4. The router updates routing rules.
5. Plugin config changes take effect on the next job dispatch for that plugin.
6. Newly added plugins are discovered and `init` is run.
7. Removed plugins: queued jobs for them are cancelled (status → `dead`, reason: "plugin removed"). No new jobs are enqueued.
8. Disabled plugins (`enabled: false`): same as removed — cancel queued, stop scheduling.

**CLI mechanism:** `ductile reload` reads the PID from the lock file and sends `SIGHUP`. If the PID file doesn't exist or the process isn't running → error.

---

### 14. Dispatch: Serial, Single Lane

**No priority lanes. No concurrency. One job at a time, FIFO.**

This is a personal integration server processing maybe 50 jobs per day. Two-lane dispatch adds goroutine coordination, queue selection logic, and potential starvation of the "normal" lane. That complexity buys nothing when your total job runtime is under 5 minutes per day.

**If a webhook-triggered notification is stuck behind a slow poll:** the poll will time out (Decision 4) within 60 seconds at most. That's an acceptable latency for a personal server. If it's not acceptable, the real fix is reducing the poll timeout, not adding dispatch complexity.

**Revisit condition:** If daily job count exceeds 500, or median queue wait time exceeds 30 seconds, consider adding a second lane. Not before.

---

### 15. Routing Semantics

**Fan-out:** A single event can match multiple routes. All matching routes produce a job. Routes are evaluated in config order but all matches fire — it's not first-match-wins.

**No match:** If an event matches no route, it is logged at DEBUG level and dropped. This is normal — not every event needs a downstream consumer. No error, no dead-letter.

**Event type matching:** Exact string match only. No wildcards, no regexes, no glob patterns. If you want a plugin to receive multiple event types, declare multiple routes.

**No conditional filters in V1.** No `payload.severity == "high"` filtering. If you need conditional logic, put it in the receiving plugin — it can inspect the payload and no-op if it's not relevant. Plugins are cheap (spawn-per-command, exits in milliseconds). Don't put a query language in the router.

---

### 16. Plugin Trust & Execution

- Plugins MUST live under `plugins_dir`. Symlinks are resolved and must resolve to within `plugins_dir`.
- Each plugin MUST have a `manifest.yaml` with an explicit `entrypoint` field.
- The core constructs the execution path as `<plugins_dir>/<plugin_name>/<entrypoint>`. No path traversal — `..` in `entrypoint` is rejected.
- The entrypoint MUST be executable (`chmod +x`). The core does not invoke interpreters — the shebang line handles that.
- World-writable plugin directories are refused at load time (log error, skip plugin).
- Plugins run as the same OS user as the core. Use systemd `User=ductile` to limit blast radius.

---

### 17. Jitter Behavior

Jitter is computed **per scheduled run**, not per tick.

When the scheduler determines a plugin is due, it computes the next run time as:

```
next_run = last_successful_run + interval + random(-jitter/2, +jitter/2)
```

The jitter offset is fixed for that scheduled run. It does not re-randomize on each tick. This prevents schedule "wander" where a plugin drifts further and further from its nominal time.

`preferred_window` is a hard constraint: if `next_run` falls outside the window, it snaps to the start of the next valid window. Jitter cannot push a run outside the preferred window.

---

### 18. Logging & stderr

**Core logs:** JSON, to stdout. Fields: `timestamp`, `level`, `component`, `plugin` (when relevant), `job_id` (when relevant), `message`.

**Plugin stderr:** Captured. Always. Stored in the `job_log` record for that job, capped at 64 KB. Tagged with the plugin name. Logged at WARN level to the core's log stream.

**Plugin stdout:** Reserved exclusively for the protocol response. Anything that isn't valid JSON on stdout is a protocol error — job fails, stderr + stdout are captured for debugging.

**Redaction:** Not in V1. The operator is responsible for not logging secrets in plugin stderr. A future version may support configurable redaction patterns.

---

### 19. Job Log Retention

`job_log` is pruned on every scheduler tick:

```sql
DELETE FROM job_log WHERE completed_at < datetime('now', '-30 days')
```

30 days is the default. Configurable:

```yaml
service:
  job_log_retention: 30d
```

No per-plugin retention. No row-count limits. Time-based is simple and predictable. At 50 jobs/day, 30 days is ~1500 rows — trivial for SQLite.

---

### 20. Core Health Endpoint

**`/healthz` on the webhook listener port.** Returns:

```json
{
  "status": "ok",
  "uptime_seconds": 3600,
  "queue_depth": 2,
  "plugins_loaded": 5,
  "plugins_circuit_open": 0
}
```

No authentication. It's on localhost. If fronted by a proxy, the proxy can restrict access.

Useful for: systemd watchdog (`Type=notify` with `WatchdogSec`), monitoring scripts, quick operator checks.

---

## Summary of Changes to RFC-001

| RFC-001 Element | Change |
|-----------------|--------|
| Job structure | Expanded: added `status`, `attempt`, `max_attempts`, `dedupe_key`, `next_retry_at`, `last_error`, `parent_job_id`. Removed `priority`. |
| Plugin manifest | Added `protocol`, `entrypoint`. Split `config_keys` into `required`/`optional`. |
| Plugin protocol | Versioned. Request adds `protocol`, `job_id`, `deadline_at`. Response adds `retry`, structured `error`. |
| `plugin_state` table | Changed from key-value to single JSON blob per plugin. |
| Open questions | All resolved: timeouts (Decision 4), stderr (Decision 18), reload (Decision 13), multi-instance (Decision 12), versioning (Decision 7), OAuth (Decision 10). |
| Config example | Added per-plugin `retry` and `timeouts` sections. Webhook `signature_header` and `max_body_size`. Service `job_log_retention`. |

---

## Feedback Requested

This RFC makes 20 opinionated decisions. For each, the question is not "is this the best possible design" but "is this good enough to build against, and will it avoid the class of bugs that make integration gateways feel haunted?"

Specific challenges welcome on:
1. Is at-least-once the right delivery guarantee, or should specific plugins get at-most-once semantics?
2. Is the circuit breaker (5 consecutive failures, 30m reset) too aggressive or too lenient?
3. Is spawn-per-command viable for plugins that need to maintain persistent connections (e.g., WebSocket listeners)?
4. Should the core eventually support a "streaming" plugin mode for long-lived consumers, or is that out of scope forever?
