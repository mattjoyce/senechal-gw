# astro_rebuild_staging

Rebuild and restart a local Astro staging site by running a configured shell command. This is a `sys_exec`-style plugin fork intended for one-off operational tasks.

## Commands
- `handle` (write): Execute the configured shell command.
- `health` (read): Validate the command and runtime prerequisites.

## Configuration
Required:
- `command`: Shell command to execute.

Optional:
- `event_type`: Event type to emit (default: `sys_exec.completed`).
- `working_dir`: Working directory for the command.
- `timeout_seconds`: Command timeout (seconds).
- `env`: Map of extra environment variables.
- `retry_on_exit_codes`: List of exit codes that should be retried.
- `emit_event`: Emit an event after execution (default: true).
- `include_output_in_event`: Include stdout/stderr in event payload.
- `stdout_max_bytes`, `stderr_max_bytes`: Truncation limits.

Incoming payload fields are exposed as environment variables with the
`DUCTILE_PAYLOAD_` prefix (e.g. `payload.title` → `DUCTILE_PAYLOAD_TITLE`).

## Events
Emits `event_type` with payload including `command`, `exit_code`, `duration_ms`,
`working_dir`, `executed_at`, and optionally `stdout`/`stderr`.

## Example
```yaml
plugins:
  astro_rebuild_staging:
    enabled: true
    config:
      command: "docker compose -f ~/admin/docker-compose.yml up -d --build"
      working_dir: "~/admin"
```
