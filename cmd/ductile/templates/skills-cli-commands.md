## CLI Commands

> Operator guidance: use `--json` for structured output; use `--dry-run` before
> any mutation; prefer `config check --json` as the first diagnostic step.

Format: `<command> tier=<READ|WRITE> mut=<0|1> out=<human|json|tui> [flags="<...>"] d="<intent>"`

- config.check tier=READ mut=0 out=human|json flags="[--json]" d="Validate syntax, policy, and integrity checksums."
- config.lock tier=WRITE mut=1 out=human d="Authorize current state by regenerating .checksums."
- config.show tier=READ mut=0 out=human|json flags="[entity]" d="View resolved configuration for an entity."
- config.get tier=READ mut=0 out=human|json flags="<path>" d="Read a specific config value by dotted path."
- config.set tier=WRITE mut=1 out=human|json flags="<path>=<val> [--dry-run] [--apply]" d="Update config; dry-run first."
- system.status tier=READ mut=0 out=human|json flags="[--json]" d="Check gateway health and PID lock."
- system.reset tier=WRITE mut=1 out=human flags="<plugin>" d="Reset a tripped circuit breaker."
- system.watch tier=READ mut=0 out=tui d="Open real-time diagnostic dashboard (Overwatch)."
- system.skills tier=READ mut=0 out=markdown flags="[--config <dir>]" d="Export capability manifest."
- job.inspect tier=READ mut=0 out=human|json flags="<job_id>" d="Retrieve logs, baggage, and workspace artifacts for a job."
