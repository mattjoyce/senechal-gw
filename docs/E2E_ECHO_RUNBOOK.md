# Echo Plugin E2E Validation Runbook (ID 20)

This runbook validates the Sprint 1 MVP end-to-end loop:

scheduler -> SQLite queue -> dispatch -> plugin subprocess -> protocol v1 response -> state store -> job completion + logs

Dependencies (must be merged to `main`):
- Scheduler (Agent 3)
- Dispatch loop (Agent 1)
- Queue + state store + PID lock + SQLite wrapper (Agent 2)

## Validation Checklist

1. Config loads successfully.
- Start should parse `config.yaml` without errors.

2. Plugin discovered and validated.
- `plugins/echo/manifest.yaml` loads.
- Entrypoint `plugins/echo/run.sh` is executable.
- Manifest validates protocol=1, commands include `poll`.

3. Scheduler enqueues `poll` job on interval.
- On tick, a `queued` row appears in `job_queue` for plugin `echo` + command `poll`.

4. Dispatch dequeues and executes plugin.
- A `queued` job transitions to `running`.
- Dispatch spawns `plugins/echo/run.sh` and writes protocol v1 request JSON to stdin.

5. Plugin response parsed correctly.
- Response JSON decodes (strictly) as protocol v1.
- `status` is `ok` or `error`.

6. State updated in SQLite.
- `plugin_state.state.last_run` is updated with a UTC timestamp.

7. Logs captured.
- Plugin-provided `response.logs[]` are emitted in core logs.

8. Job marked complete.
- `job_queue.status` becomes terminal (`succeeded`/`failed`/`timed_out`/`dead`).
- `job_log` contains an entry for the job attempt.

## Manual Test Steps

### Preflight

1. Confirm plugin validates and is executable:
```sh
ls -la plugins/echo
cat plugins/echo/manifest.yaml
```

2. Start the service:
```sh
senechal-gw start --config config.yaml
```

3. Identify the SQLite DB path from config (example from `MVP.md` uses `./data/state.db`).

### Happy Path: plugin succeeds

1. Run until at least one scheduler tick enqueues a job.
2. Verify queue -> running -> succeeded:
```sh
sqlite3 ./data/state.db 'select id, plugin, command, status, attempt, created_at, started_at, completed_at, last_error from job_queue order by created_at desc limit 5;'
sqlite3 ./data/state.db 'select plugin_name, updated_at, state from plugin_state where plugin_name = "echo";'
sqlite3 ./data/state.db 'select id, plugin, command, status, attempt, completed_at, last_error from job_log order by completed_at desc limit 5;'
```

Expected:
- Job terminal status is `succeeded`.
- `plugin_state.state` contains a `last_run` timestamp.

### Error Path: plugin returns `status=error`

Set plugin config to force error:
- In `config.yaml`, set `plugins.echo.config.mode: error`.

Restart and verify:
- Dispatch completes the job as `failed`.
- `job_log.last_error` is populated.

### Timeout Path: plugin hangs

Set plugin config:
- `plugins.echo.config.mode: hang`
- Ensure `plugins.echo.timeouts.poll` is small (ex: `2s`).

Restart and verify:
- Job becomes `timed_out`.
- Process is terminated (SIGTERM then SIGKILL after grace).

### Protocol Error: plugin returns invalid JSON

Set plugin config:
- `plugins.echo.config.mode: protocol_error`

Restart and verify:
- Dispatch records a protocol decode error.
- Job is marked `failed` (or equivalent terminal failure).

### Crash Recovery: orphaned job handled

1. Let a job transition to `running`.
2. Kill the core process hard:
```sh
kill -9 <pid>
```
3. Restart:
```sh
senechal-gw start --config config.yaml
```

Expected:
- On startup, any `running` jobs are detected and re-queued (attempt incremented) until `max_attempts`, then marked `dead`.
- Recovery behavior is logged at `warn`.
