# sys_exec

Execute a configured shell command and optionally emit an event with execution metadata.

## Commands
- `handle` (write): Run the configured shell command.
- `health` (read): Validate command and runtime prerequisites.

## Configuration
Required:
- `command`: Command to execute. Can be a string (which will be tokenized) or a list of arguments.
  Environment variables like `$DUCTILE_PAYLOAD_VAR` are expanded.
  Shell metacharacters (like `|`, `>`, `;`) are NOT supported in the command string.
  If you need shell features, use `["/bin/sh", "-c", "your command here"]`.

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
`DUCTILE_PAYLOAD_` prefix. These are automatically expanded if used in the `command` string or list.

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
