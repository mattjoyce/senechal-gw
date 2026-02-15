# Ductile Operator Skill Manifest

This document defines the capabilities ("Skills") of the Ductile Gateway for LLM-based operators. All interactions with the gateway should follow the **NOUN ACTION** pattern.

## Core Gateway Utilities

These skills are used to manage the gateway's state, integrity, and observability.

### Skill: system
**Purpose:** Manage gateway lifecycle and health.

- `system start [--config PATH]`: Start the gateway service.
- `system status [--json]`: Check global health (config, DB, PID lock).
- `system monitor`: Launch the real-time TUI dashboard (Human only).
- `system watch`: Alias for monitor.
- `system reset <plugin>`: Manually reset a plugin's circuit breaker.

### Skill: config
**Purpose:** Manage system configuration and integrity.

- `config check [--json]`: Validate syntax, policy, and integrity hashes.
- `config lock`: Authorize current state (re-generate BLAKE3 hashes).
- `config show [entity] [--json]`: View the fully resolved configuration.
- `config get <path> [--json]`: Read a specific configuration value (e.g., `service.name`).
- `config set <path>=<value> [--dry-run | --apply]`: Safely update configuration.
- `config token create --name <name> [--tui]`: Create a new scoped API token.

### Skill: job
**Purpose:** Inspect execution history and lineage.

- `job inspect <job_id> [--json]`: Retrieve logs, baggage, and workspace artifacts for a specific job.

---

## Plugin Skills (Auto-generated)

> **Note:** To re-generate this section with the latest plugin registry, run:  
> `ductile system skills >> SKILL.md` (and remove the old section).

The following skills are available based on currently registered plugins. Use `trigger` via API to invoke them.

### echo
**Description:** A demonstration plugin that echoes input and provides health status.

**Actions:**
- `poll`: [WRITE] Emits echo.poll events and updates last_run.
- `health`: [READ] Returns current health status and version.

---

## Operator Rules
1. **Prefer JSON:** Always use `--json` for inspection actions to ensure structured reasoning.
2. **Integrity First:** If configuration is modified, you MUST run `config lock` before restarting the service.
3. **Dry-run Mutations:** Use `--dry-run` with `config set` to validate changes before application.
4. **Lineage Tracking:** Use `job inspect` to understand why a downstream pipeline was triggered.
