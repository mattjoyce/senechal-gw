## CLI Commands

> RFC-004 alignment: use `--json` for structured output; use `--dry-run` before
> any mutation; prefer `config check --json` as the first diagnostic step.

### config

- **`config check [--json]`** — Validate syntax, policy, and integrity checksums.
- **`config lock`** — Authorize current state (re-generate `.checksums`).
- **`config show [entity]`** — View resolved configuration for an entity.
- **`config get <path>`** — Read a specific config value by dotted path.
- **`config set <path>=<val> [--dry-run] [--apply]`** — Update config (dry-run first).

### system

- **`system status [--json]`** — Check gateway health and PID lock.
- **`system reset <plugin>`** — Reset a tripped circuit breaker.
- **`system watch`** — Real-time diagnostic TUI (Overwatch).
- **`system skills [--config <dir>]`** — This command. Re-run with config for full manifest.

### job

- **`job inspect <job_id>`** — Retrieve logs, baggage, and workspace artifacts for a job.
