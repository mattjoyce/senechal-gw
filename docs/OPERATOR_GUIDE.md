# Operator Guide

This guide is intended for system administrators and LLM operators managing a Ductile instance. It covers day-to-day operations, monitoring, and administrative safety.

---

## 1. System Operations

### Starting the Service
The primary way to run Ductile is in the foreground:
```bash
./ductile system start
```
For production environments, we recommend using a **systemd** unit. See [Architecture](ARCHITECTURE.md#14-deployment) for an example configuration.

### Reloading Configuration
You can reload the configuration without restarting the service by sending a `SIGHUP` signal:
```bash
# This command is planned for a future release
./ductile system reload
```

---

## 2. Monitoring & Observability

### Real-Time Dashboard (TUI)
Ductile includes a built-in terminal UI for real-time visibility:
```bash
./ductile system monitor --api-key "your-admin-token"
```
The monitor shows:
-   Service health and uptime.
-   A live process tree of active and recent jobs.
-   A real-time event stream.

### Logging
Ductile emits structured JSON logs to `stdout`. These are ideal for consumption by Logstash, Fluentd, or simple `jq` queries.
```bash
./ductile system start | jq 'select(.level == "ERROR")'
```

### SSE Event Stream
For custom monitoring tools, subscribe to the live event stream:
```bash
curl -N -H "Authorization: Bearer <token>" http://localhost:8080/events
```

---

## 3. Configuration Management

Ductile uses a tiered directory model for configuration. The source of truth is typically `~/.config/ductile/`.

### Administrative Commands
Use the `config` noun for surgical administration:
-   **Show resolved config:** `ductile config show` (includes all defaults and merges).
-   **Get a specific value:** `ductile config get plugins.echo.enabled`.
-   **Set a value safely:** `ductile config set plugins.echo.enabled=false --apply`.

### Operational Integrity (Lock & Check)
To prevent unauthorized modifications to sensitive files (like `tokens.yaml` or `webhooks.yaml`), Ductile uses **BLAKE3** hash verification.

1.  **Authorize changes:** After editing config files, update the hashes:
    ```bash
    ductile config lock
    ```
2.  **Validate state:** Ductile runs an automatic check at startup. You can run it manually with:
    ```bash
    ductile config check
    ```

### Strict Mode
For hardened environments, enable `service.strict_mode: true` in your `config.yaml`.
In strict mode:
- The system **will not start** if any file fails integrity verification (no warnings).
- The system **will not start** if any configuration check fails (e.g., missing dependencies).
- The system **requires** at least one API token to be defined if the API is enabled.

### Managing Scoped Tokens (TUI)
You can interactively create scoped API tokens using the built-in wizard:
```bash
./ductile config token create --name "my-service" --tui
```
This TUI reads the manifests of all enabled plugins to present you with a checkbox list of available scopes (e.g., `jobs:ro`, `plugin:echo:rw`).

---

## 4. API Reference

Ductile provides a REST API for programmatic control. By default, it listens on `localhost:8080`.

### Manual Triggering
You can manually enqueue any plugin command via the API:
```bash
curl -X POST http://localhost:8080/trigger/echo/poll 
  -H "Authorization: Bearer <token>" 
  -H "Content-Type: application/json" 
  -d '{"payload": {"message": "Hello from API"}}'
```

### Job Inspection
Retrieve the status and results of any job:
```bash
curl http://localhost:8080/job/<job_id> -H "Authorization: Bearer <token>"
```

For a full list of endpoints and schemas, see the [API Reference](API_REFERENCE.md).

---

## 5. Troubleshooting

-   **Failed to acquire PID lock:** Another instance is running. Check `ps aux | grep ductile`.
-   **Plugin not running:** Ensure it is `enabled: true` in `config.yaml` and has a valid `schedule`.
-   **Database is locked:** SQLite concurrency limit. Ductile uses WAL mode to mitigate this, but very high API volume may still trigger it.
-   **Tampering detected:** Configuration file was modified without running `config lock`. Run `ductile config lock` if the change was intentional.
