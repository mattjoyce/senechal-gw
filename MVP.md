# Senechal Gateway — MVP

**Goal:** Prove the core loop works end-to-end. Scheduler submits a job to the queue, dispatch spawns a plugin, plugin reads input and produces output, state is persisted.

**Spec subset:** This implements the minimum from SPEC.md needed to validate the architecture. Everything not listed here is out of scope for MVP.

---

## What the MVP does

1. Reads `config.yaml` — loads service settings and one plugin definition.
2. Opens SQLite database — creates tables if missing.
3. Acquires PID file lock — single instance only.
4. Runs scheduler tick loop — checks if the plugin is due, enqueues a `poll` job.
5. Dispatch loop pulls the job, spawns the plugin subprocess.
6. Sends JSON request on stdin, reads JSON response from stdout.
7. Persists `state_updates` back to SQLite.
8. Logs structured JSON to stdout.
9. Handles crash recovery on restart (orphaned `running` jobs re-queued).

---

## What the MVP does NOT do

- Routing / event fan-out
- Webhooks
- Circuit breaker
- Deduplication
- Retry with backoff (jobs run once; failures logged)
- Config reload (SIGHUP)
- Health endpoint
- CLI commands beyond `start`
- Poll guard (only one plugin, not needed yet)

---

## Components to build

### 1. Config loader
- Parse `config.yaml`
- Interpolate `${ENV_VAR}` syntax
- Validate: plugin has `schedule`, required fields present
- No reload support yet

### 2. SQLite state store
- Create `plugin_state` and `job_queue` tables on first run
- Read state for a plugin → return JSON blob
- Write state updates → shallow merge

### 3. Plugin discovery
- Scan `plugins_dir` for directories containing `manifest.yaml`
- Parse manifest: `name`, `protocol`, `entrypoint`, `commands`, `config_keys`
- Validate: `protocol` is `1`, `entrypoint` exists and is executable, required config keys present

### 4. Scheduler
- Tick loop at `tick_interval` (default 60s)
- On each tick: is `now >= next_run` for the plugin? If yes, enqueue a `poll` job.
- Compute `next_run` using jitter (per SPEC section 4.2)
- Prune `job_log` on each tick

### 5. Work queue
- SQLite-backed
- Enqueue: insert row with `status = queued`
- Dequeue: select oldest `queued` job, set `status = running`
- Complete: set `status = succeeded` or `status = failed`, write to `job_log`

### 6. Dispatch loop
- Pull one job from queue
- Spawn plugin: `<plugins_dir>/<plugin_name>/<entrypoint>`
- Write request envelope to stdin (protocol v1)
- Read response from stdout
- Apply timeout: `SIGTERM` → 5s grace → `SIGKILL`
- Capture stderr
- On success: apply `state_updates`, mark `succeeded`
- On failure: mark `failed`, store `last_error`
- Ignore `events` array (routing not implemented)

### 7. PID file lock
- `flock(LOCK_EX | LOCK_NB)` on `<state_dir>/senechal-gw.lock`
- Write PID
- Fail fast if lock not acquired

### 8. Crash recovery
- On startup: find `status = running` jobs, increment `attempt`, re-queue if under `max_attempts`, else `dead`
- Aligns with SPEC.md operational semantics (at-least-once delivery)
- Log each orphan at WARN

### 9. Logging
- JSON to stdout: `timestamp`, `level`, `component`, `plugin`, `job_id`, `message`

### 10. CLI: `start` command only
- `senechal-gw start --config <path>`
- Foreground process, ctrl-C to stop

---

## Test plugin: `echo`

A trivial plugin to validate the loop:

```
plugins/echo/
├── manifest.yaml
└── run.sh
```

**manifest.yaml:**
```yaml
name: echo
version: 0.1.0
protocol: 1
entrypoint: run.sh
description: "Reads input, echoes it back with a timestamp"
commands: [poll]
config_keys:
  required: []
  optional: []
```

**run.sh:**
```bash
#!/usr/bin/env bash
input=$(cat)
echo "{\"status\": \"ok\", \"events\": [], \"state_updates\": {\"last_run\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}, \"logs\": [{\"level\": \"info\", \"message\": \"echo plugin ran\"}]}"
```

---

## MVP config

```yaml
service:
  name: senechal-gw
  tick_interval: 60s
  log_level: info
  log_format: json

state:
  path: ./data/state.db

plugins_dir: ./plugins

plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
      jitter: 1m
    config: {}
    timeouts:
      poll: 60s
```

---

## Done when

- [ ] `senechal-gw start` runs, holds PID lock, ticks on schedule
- [ ] Echo plugin is spawned on schedule with jitter
- [ ] Plugin receives valid protocol v1 JSON on stdin
- [ ] Plugin response is parsed, `state_updates` persisted to SQLite
- [ ] Stderr captured and logged
- [ ] Timeout kills a hung plugin (test with a `sleep 999` plugin)
- [ ] Crash recovery: kill -9 the process, restart, orphaned job logged
- [ ] Structured JSON logs on stdout
