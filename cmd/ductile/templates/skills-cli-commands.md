## CLI Commands

> Operator guidance: use `--json` for structured output; use `--dry-run` before
> any mutation; prefer `config check --json` as the first diagnostic step.

Format: `<command> tier=<READ|WRITE> mut=<0|1> out=<human|json|tui> [flags="<...>"] d="<intent>"`

### System & Lifecycle
- system.start tier=WRITE mut=1 out=human d="Start the gateway service in foreground."
- system.status tier=READ mut=0 out=human|json flags="[--json]" d="Check gateway health: PID lock, state DB, and plugin reachability."
- system.breaker tier=READ mut=0 out=human|json flags="<plugin> [--command <command>] [--json] [--limit <n>]" d="Inspect circuit breaker current state and transition history."
- system.reload tier=WRITE mut=1 out=human|json flags="[--json] [--api-url <url>] [--api-key <key>]" d="Hot-reload configuration without restart."
- system.reset tier=WRITE mut=1 out=human flags="<plugin>" d="Reset a tripped circuit breaker for a plugin."
- system.watch tier=READ mut=0 out=tui d="Open real-time diagnostic dashboard (Overwatch)."
- system.skills tier=READ mut=0 out=markdown flags="[--config <dir>]" d="Export live capability manifest for LLM consumption."

### Configuration Management
- config.check tier=READ mut=0 out=human|json flags="[--json] [--strict]" d="Validate syntax, policy, and integrity checksums."
- config.lock tier=WRITE mut=1 out=human d="Authorize current state by regenerating .checksums."
- config.show tier=READ mut=0 out=human|json flags="[entity] [--json]" d="View resolved configuration for an entity."
- config.get tier=READ mut=0 out=human|json flags="<path> [--json]" d="Read a specific config value by dotted path."
- config.set tier=WRITE mut=1 out=human|json flags="<path>=<val> [--dry-run] [--apply]" d="Update config value; dry-run first."
- config.init tier=WRITE mut=1 out=human flags="[--config-dir <path>] [--force]" d="Initialize a new configuration directory with defaults."
- config.backup tier=READ mut=0 out=human flags="[--output <path>]" d="Create a backup archive of the current configuration."
- config.restore tier=WRITE mut=1 out=human flags="<archive-path>" d="Restore configuration from a backup archive."

### Auth & Scopes
- config.token.create tier=WRITE mut=1 out=human|json flags="--name <n> [--scopes <csv>] [--tui]" d="Create a new scoped API token."
- config.token.list tier=READ mut=0 out=human|json d="List all registered API tokens (redacted)."
- config.token.inspect tier=READ mut=0 out=human|json flags="<name>" d="Inspect token details and verify scope integrity."
- config.token.delete tier=WRITE mut=1 out=human|json flags="<name>" d="Revoke an API token."
- config.scope.add tier=WRITE mut=1 out=human|json flags="<token> <scope>" d="Add a scope to an existing token."
- config.scope.remove tier=WRITE mut=1 out=human|json flags="<token> <scope>" d="Remove a scope from a token."
- config.scope.validate tier=READ mut=0 out=human|json flags="<scope-string>" d="Dry-run validate a scope against discovered plugins."

### Plugins, Routes & Webhooks
- config.plugin.list tier=READ mut=0 out=human|json d="List configured and discovered plugins."
- config.plugin.show tier=READ mut=0 out=human|json flags="<name>" d="Show configuration for a specific plugin."
- config.plugin.set tier=WRITE mut=1 out=human|json flags="<name> <path> <value>" d="Update plugin-specific configuration."
- config.route.list tier=READ mut=0 out=human|json d="List all event routes."
- config.route.add tier=WRITE mut=1 out=human|json flags="--from <p> --event <e> --to <p>" d="Add a new event route."
- config.route.remove tier=WRITE mut=1 out=human|json flags="--from <p> --event <e> --to <p>" d="Remove an event route."
- config.webhook.list tier=READ mut=0 out=human|json d="List configured webhooks."
- config.webhook.add tier=WRITE mut=1 out=human|json flags="--path <p> --plugin <p> --secret-ref <r>" d="Add a new webhook endpoint."

### Jobs & Execution
- plugin.list tier=READ mut=0 out=human|json d="List available plugins and their commands via API."
- plugin.run tier=WRITE mut=1 out=human|json flags="<name> [--command <c>] [--payload <json>]" d="Manually trigger a plugin command via API."
- job.inspect tier=READ mut=0 out=human|json flags="<job_id>" d="Retrieve logs, baggage, and workspace artifacts for a job."
- job.logs tier=READ mut=0 out=human|json flags="[--plugin <p>] [--query <text>] [--limit <n>]" d="Query stored job logs for audit and troubleshooting."
