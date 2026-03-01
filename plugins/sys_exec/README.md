# sys_exec

Execute a configured shell command and optionally emit an event with execution metadata.

## Commands
- `handle` (write): Run the configured shell command.
- `health` (read): Validate command and runtime prerequisites.

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
`DUCTILE_PAYLOAD_` prefix.

## Events
Emits `event_type` with payload including `command`, `exit_code`, `duration_ms`,
`working_dir`, `executed_at`, and optionally `stdout`/`stderr`.

## Example
```yaml
plugins:
  sys_exec:
    enabled: true
    config:
      command: "make deploy"
      working_dir: /srv/app
```
