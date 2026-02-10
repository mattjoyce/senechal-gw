# Senechal Gateway User Guide

## Table of Contents
1.  [Introduction](#introduction)
2.  [Getting Started](#getting-started)
3.  [Configuration](#configuration)
    *   [Multi-file Configuration](#multi-file-configuration)
    *   [Environment Variable Interpolation](#environment-variable-interpolation)
    *   [Security & Integrity (Hash Verification)](#security--integrity-hash-verification)
4.  [Webhooks](#webhooks)
    *   [Webhook Configuration](#webhook-configuration)
    *   [HMAC Verification](#hmac-verification)
    *   [Security Best Practices](#webhook-security-best-practices)
5.  [Authentication & Authorization](#authentication--authorization)
    *   [Bearer Token Authentication](#bearer-token-authentication)
    *   [API Scopes](#api-scopes)
    *   [Command Permission Types (Read vs Write)](#command-permission-types-read-vs-write)
6.  [Using the API](#using-the-api)
    *   [API Endpoints Introduction](#api-endpoints-introduction)
    *   [POST /trigger/{plugin}/{command}](#post-triggerplugincommand)
    *   [GET /job/{job_id}](#get-jobjob_id)
    *   [API Use Cases](#api-use-cases)
7.  [Core Concepts](#core-concepts)
    *   [Scheduler](#scheduler)
    *   [Plugins](#plugins)
    *   [State Management](#state-management)
    *   [Crash Recovery](#crash-recovery)
8.  [Monitoring & Observability](#monitoring--observability)
    *   [GET /healthz](#get-healthz)
    *   [GET /events (SSE)](#get-events-sse)
9.  [Configuration Reference](#configuration-reference)
10. [Plugin Development Guide](#plugin-development-guide)
    *   [Bash Plugins](#bash-plugins)
    *   [Python Plugins](#python-plugins)
    *   [Manifest Command Metadata](#manifest-command-metadata)
11. [Operations and Troubleshooting](#operations-and-troubleshooting)

## 1. Introduction
The Senechal Gateway is a lightweight, YAML-configured, and modular integration gateway designed for personal automation. It orchestrates various tasks by utilizing polyglot plugins executed via a subprocess protocol. The core idea is to provide a simple, yet extensible platform for handling event-driven workflows, ETL processes, and various integrations without the overhead of larger, more complex integration servers.

This user guide provides comprehensive documentation to help you:
-   Understand the core concepts and architecture of the Senechal Gateway.
-   Install, configure, and operate the gateway effectively.
-   Develop and integrate your own custom plugins in various programming languages (e.g., Bash, Python).
-   Troubleshoot common issues and monitor the gateway's operation.

Whether you're looking to automate daily tasks, connect disparate services, or build custom integrations, the Senechal Gateway offers a flexible and robust solution.

## 2. Getting Started

### Installation

To install and run the Senechal Gateway, you will need to have Go version 1.25.4 or newer installed on your system.

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/mattjoyce/senechal-gw.git
    cd senechal-gw
    ```

2.  **Build the gateway:**
    ```bash
    go build -o senechal-gw ./cmd/senechal-gw
    ```

    This will create an executable named `senechal-gw` in your project root directory.


### Basic Usage

After building the `senechal-gw` executable, you can start the gateway.

1.  **Ensure plugin directory exists:**
    The gateway expects a `plugins` directory in the root. The `echo` plugin is provided as an example.
    ```bash
    ls -F plugins/echo/manifest.yaml
    # Should output: plugins/echo/manifest.yaml
    ```

2.  **Configure the `echo` plugin (optional):**
    The `config.yaml` file in the project root defines the gateway's settings. By default, the `echo` plugin is enabled and scheduled to run every 5 minutes. You can customize its schedule or add plugin-specific configuration under the `plugins.echo.config` section.

    ```yaml
    # config.yaml excerpt
    plugins:
      echo:
        enabled: true
        schedule:
          every: 5m
          jitter: 30s
        # ... other settings ...
        config:
          message: "Hello from Senechal Gateway!"
    ```

3.  **Start the Senechal Gateway:**
    Run the gateway from the project root directory.
    ```bash
    ./senechal-gw start
    ```
    The gateway will start and begin logging its operations. You should see log entries indicating the scheduler is running and, after the configured interval, the `echo` plugin executing.

    Example log output (abbreviated):
    ```json
    {"level":"info","msg":"Senechal Gateway starting...","time":"..."}
    {"level":"info","plugin_name":"echo","schedule":"5m","msg":"scheduling plugin","time":"..."}
    # ... after 5 minutes ...
    {"level":"info","job_id":"...","plugin_name":"echo","status":"started","msg":"plugin job started","time":"..."}
    {"level":"info","job_id":"...","plugin_name":"echo","output":"Hello from Senechal Gateway!","status":"completed","msg":"plugin job completed","time":"..."}
    ```
    Press `Ctrl+C` to stop the gateway gracefully.

## 3. Configuration
The Senechal Gateway uses a flexible, multi-file configuration system that allows for clean separation of concerns, environment-specific overrides, and secure handling of sensitive credentials.

### Multi-file Configuration
While you can keep your entire configuration in a single `config.yaml` file, complex setups benefit from splitting definitions into multiple files. This is achieved using the `include` array in your main configuration file.

The gateway looks for configuration in the following priority order:
1.  **--config-dir** flag
2.  **SENECHAL_CONFIG_DIR** environment variable
3.  **~/.config/senechal-gw/** (Recommended)
4.  **/etc/senechal-gw/**
5.  **./config.yaml** (Legacy/Development fallback)

#### Example Multi-file Structure
A typical production setup might look like this in `~/.config/senechal-gw/`:
```
.config/senechal-gw/
├── config.yaml          # Main entry point
├── plugins.yaml         # Plugin schedules and settings
├── webhooks.yaml        # Webhook endpoint definitions
├── tokens.yaml          # API tokens and secrets (Sensitive)
└── .checksums.json      # Integrity hashes for sensitive files
```

**Main `config.yaml`:**
```yaml
service:
  name: my-gateway
  log_level: info

include:
  - plugins.yaml
  - webhooks.yaml
  - tokens.yaml
```

#### API Service Configuration
To enable the management API, add an `api` section to your `config.yaml`:

```yaml
api:
  enabled: true          # Set to true to enable the HTTP API
  listen: "0.0.0.0:8080" # Address and port for the API listener
  auth:
    api_key: ${API_KEY}  # Legacy support (admin scope)
    tokens: []           # Scoped tokens (preferred)
```

### Environment Variable Interpolation
Configuration values support environment variable interpolation using the `${VAR_NAME}` syntax. This is critical for keeping secrets out of version control and for environment-specific pathing.

```yaml
# Environment-specific include
include:
  - ${ENVIRONMENT:-prod}/plugins.yaml

plugins:
  github:
    config:
      api_token: ${GITHUB_TOKEN}
```
Interpolation is performed during the loading phase, before YAML parsing, allowing you to use variables in keys, values, and include paths.

### Security & Integrity (Hash Verification)
To prevent unauthorized modifications to sensitive configuration files (like `tokens.yaml` or `webhooks.yaml`), Senechal Gateway implements a BLAKE3-based integrity check.

When multi-file mode is used, the gateway verifies that the hashes of "scope files" (detected by name: `tokens.yaml`, `webhooks.yaml`) match those stored in `.checksums.json`. If a file has been modified without updating the hash, the gateway will refuse to start.

**Updating Hashes:**
If you intentionally modify a sensitive configuration file, you must update the checksums:
```bash
senechal-gw config hash-update --config-dir ~/.config/senechal-gw
```
*Why this matters:* This protects against "configuration injection" attacks where an attacker with limited filesystem access might attempt to add a backdoor API token or redirect a webhook.

## 4. Webhooks
The Senechal Gateway can act as a webhook listener, allowing external services (like GitHub, GitLab, or Stripe) to trigger internal plugin commands directly via HTTP POST requests.

### Webhook Configuration
Webhooks are defined in the `webhooks` section of your configuration. It is recommended to keep these in a separate `webhooks.yaml` file.

```yaml
# webhooks.yaml
webhooks:
  listen: ":8081"  # Separate port for webhooks (optional)
  endpoints:
    - path: /webhook/github-push
      plugin: github-handler
      command: handle
      secret: ${GITHUB_WEBHOOK_SECRET}
      signature_header: "X-Hub-Signature-256"
      max_body_size: 2MB
```

-   **path:** The URL path the gateway listens on.
-   **plugin/command:** The target to trigger when a valid request arrives.
-   **secret:** The HMAC secret used to verify the sender.
-   **signature_header:** The HTTP header containing the HMAC signature.
-   **max_body_size:** Safety limit for inbound payloads.

### HMAC Verification
Security is paramount for webhooks. Senechal Gateway requires HMAC-SHA256 signature verification for all webhook endpoints. It supports both plain hex signatures and GitHub-style `sha256=<hex>` formats.

When a request arrives, the gateway:
1.  Reads the raw request body.
2.  Computes the HMAC-SHA256 hash using the configured `secret`.
3.  Performs a constant-time comparison against the value in the `signature_header`.

If verification fails, the gateway returns `401 Unauthorized` and does *not* trigger the plugin.

### Webhook Security Best Practices
-   **Use HTTPS:** Always terminate TLS (via Nginx or Caddy) before the gateway to protect the HMAC secret and payload.
-   **Secret Rotation:** Regularly rotate your webhook secrets and update the gateway configuration.
-   **Separate Port:** Use a different port or even a different network interface for webhooks compared to the management API.
-   **Least Privilege:** Ensure the target plugin command is designed to handle untrusted input safely.

## 5. Authentication & Authorization
The Senechal Gateway uses a scope-based Bearer token authentication system to control access to its API and events.

### Bearer Token Authentication
All requests to the management API (except `/healthz`) must include an `Authorization` header:

```http
Authorization: Bearer <your-token-here>
```

Tokens are defined in your configuration (usually `tokens.yaml`):

```yaml
# tokens.yaml
api:
  auth:
    tokens:
      - token: ${MONITOR_TOKEN}
        scopes: ["plugin:ro", "events:ro"]
      - token: ${ADMIN_TOKEN}
        scopes: ["*"]
```

### API Scopes
Scopes define what an authenticated "Principal" is allowed to do.

| Scope | Description |
| :--- | :--- |
| `*` | Full administrative access (Superuser) |
| `plugin:ro` | Read-only access to plugins and command execution (read commands only) |
| `plugin:rw` | Ability to trigger any plugin command (implies `plugin:ro`) |
| `jobs:ro` | Ability to view job status and results |
| `jobs:rw` | Ability to manage or delete jobs (implies `jobs:ro`) |
| `events:ro` | Ability to stream system events via the `/events` endpoint |

### Command Permission Types (Read vs Write)
To support fine-grained authorization, plugin commands are classified as either `read` or `write`.
-   **Read Commands:** Informational commands that do not change state (e.g., `health`, `status`). These can be invoked by tokens with the `plugin:ro` scope.
-   **Write Commands:** Actions that mutate state, perform side effects, or fetch data (e.g., `poll`, `handle`, `sync`). These require the `plugin:rw` or `*` scope.

By default, the `health` command is considered `read`, and all others are `write`. You can override this in the plugin's `manifest.yaml` (see [Plugin Development Guide](#manifest-command-metadata)).

## 6. Using the API
The Senechal Gateway provides a simple HTTP API to programmatically trigger plugins and retrieve job results. This is particularly useful for external systems, such as LLM agents or custom scripts, that need to interact with the gateway on-demand rather than relying solely on scheduled tasks.

### API Endpoints Introduction
The API is organized around plugins and jobs. All requests (except `/healthz`) require authentication.

### POST /trigger/{plugin}/{command}
This endpoint allows you to enqueue a job for a specific plugin and command. The API call returns immediately with a job ID, and the plugin execution happens asynchronously.

-   **Purpose:** Enqueue jobs on-demand.
-   **Authentication:** Requires a Bearer token with `plugin:rw` or `*` scope (or `plugin:ro` for read commands).
-   **URL Parameters:**
    *   `{plugin}`: The name of the plugin to execute (e.g., `echo`, `my-custom-plugin`).
    *   `{command}`: The command to execute on the plugin (e.g., `poll`, `handle`).
-   **Request Body:** An optional JSON payload that will be passed to the plugin as part of its `config` in the request envelope.
-   **Response (202 Accepted):**
    ```json
    {
      "job_id": "uuid-v4-string",
      "status": "queued",
      "plugin": "plugin_name",
      "command": "command_name"
    }
    ```
-   **Error Responses:**
    *   `401 Unauthorized`: Missing or invalid API key.
    *   `403 Forbidden`: Insufficient scope for the requested command.
    *   `400 Bad Request`: Plugin not found or command not supported.
    *   `500 Internal Server Error`: Failed to enqueue job.

#### Example: Triggering the `echo` plugin
```bash
API_KEY="your_api_key_here" # Replace with your actual API key
curl -X POST http://localhost:8080/trigger/echo/poll \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"message": "Triggered via API"}'
```

### GET /job/{job_id}
This endpoint allows you to retrieve the current status and, upon completion, the results of a previously triggered job.

-   **Purpose:** Check job status and retrieve execution results.
-   **Authentication:** Requires a Bearer token with `jobs:ro`, `jobs:rw`, or `*` scope.
-   **URL Parameters:**
    *   `{job_id}`: The UUID of the job, as returned by the `POST /trigger` endpoint.
-   **Response (200 OK):**
    *   **Queued Job:**
        ```json
        {
          "job_id": "uuid-v4-string",
          "status": "queued",
          "plugin": "plugin_name",
          "command": "command_name",
          "created_at": "ISO8601-timestamp"
        }
        ```
    *   **Running Job:**
        ```json
        {
          "job_id": "uuid-v4-string",
          "status": "running",
          "plugin": "plugin_name",
          "command": "command_name",
          "started_at": "ISO8601-timestamp"
        }
        ```
    *   **Completed Job:**
        ```json
        {
          "job_id": "uuid-v4-string",
          "status": "completed",
          "plugin": "plugin_name",
          "command": "command_name",
          "result": { /* Plugin's JSON response from stdout */ },
          "started_at": "ISO8601-timestamp",
          "completed_at": "ISO8601-timestamp"
        }
        ```
-   **Error Responses:**
    *   `401 Unauthorized`: Missing or invalid API key.
    *   `404 Not Found`: Job ID not found.

#### Example: Polling for job completion
```bash
API_KEY="your_api_key_here" # Replace with your actual API key
JOB_ID="your_job_id_from_trigger_response" # Replace with actual job ID

# Poll for job status
curl http://localhost:8080/job/$JOB_ID \
  -H "Authorization: Bearer $API_KEY"
```

### API Use Cases
-   **LLM Tool Calling:** Integrate Senechal Gateway as a tool for Large Language Models (LLMs), allowing them to trigger actions (e.g., "check my calendar," "sync health data") programmatically.
-   **External Automation Scripts:** Use custom scripts, cron jobs, or other automation tools to interact with Senechal Gateway.
-   **Manual Testing:** Developers can quickly trigger plugins via `curl` for testing and debugging without waiting for scheduler intervals.
-   **Webhook-style Triggers:** Other services can send requests to the `/trigger` endpoint to initiate workflows in Senechal Gateway.

## 7. Core Concepts

### Scheduler
The Senechal Gateway includes a built-in scheduler responsible for orchestrating the periodic execution of configured plugins. It operates on a "tick loop" that runs at a configurable interval (defaulting to `service.tick_interval`, typically 60 seconds).

For each enabled plugin with a defined `schedule`, the scheduler calculates its next execution time using a "fuzzy interval" approach. This approach incorporates a fixed jitter to the scheduled interval. The jitter (e.g., `jitter: 30s`) introduces a random delay within a specified window, preventing all scheduled plugins from attempting to run simultaneously, thus avoiding a "thundering herd" problem and distributing load more evenly.

When a plugin's scheduled time arrives, the scheduler enqueues a `poll` job for that plugin into the system's work queue. This `poll` job initiates the plugin's execution flow. The scheduler also ensures "at-least-once" job execution semantics by utilizing deduplication keys.

The `schedule.every` field supports various named intervals suchs as `5m`, `15m`, `30m`, `hourly`, `2h`, `6h`, `daily`, `weekly`, and `monthly`.

In addition to scheduling, the tick loop is also responsible for pruning completed job logs based on the configured `job_log_retention` policy.

### Plugins
Plugins are the core extensible components of the Senechal Gateway. They are external executables designed to perform specific tasks, ranging from fetching data from external services (`poll` command) to processing events (`handle` command). A key design principle is their polyglot nature: plugins can be written in any language, as long as they adhere to a simple JSON-over-stdin/stdout communication protocol.

#### Plugin Lifecycle: Spawn-Per-Command
The Senechal Gateway operates on a "spawn-per-command" model for plugins. This means that for every job requiring a plugin (e.g., a scheduled `poll` or an event `handle`), the gateway spawns a new process for that plugin's entrypoint. This approach offers several benefits:
-   **Language Agnostic:** Plugins can be written in Bash, Python, Go, or any other language.
-   **Fault Isolation:** Failures in one plugin do not affect the core gateway or other plugins.
-   **Resource Management:** No long-lived plugin processes, preventing memory leaks and simplifying resource cleanup.

#### Plugin Directory Structure
Plugins are organized within the `plugins` directory (configurable via `plugins_dir` in `config.yaml`). Each plugin resides in its own subdirectory, containing its manifest and entrypoint script:

```
plugins/
├── my-plugin/
│   ├── manifest.yaml
│   └── run.sh (or run.py, run.exe, etc.)
└── another-plugin/
    ├── manifest.yaml
    └── run.py
```

#### Plugin Manifest (`manifest.yaml`)
Each plugin *must* have a `manifest.yaml` file in its root directory. This file provides metadata about the plugin:

```yaml
name: echo                # Unique name of the plugin
version: 1.0.0            # Plugin version
protocol: 1               # Protocol version the plugin expects (must match gateway)
entrypoint: run.sh        # The executable script to run for plugin commands
description: "A simple echo plugin"
commands: [poll, handle]  # Commands this plugin supports (poll, handle, health, init)
config_keys:
  required: []            # List of required configuration keys
  optional: [message]     # List of optional configuration keys
```
The `protocol` field is crucial; a mismatch will prevent the plugin from being loaded. The `entrypoint` must be an executable file.

#### Plugin Communication Protocol (v1)
The gateway communicates with plugins by sending a single JSON object to the plugin's `stdin` and expecting a single JSON object in response on the plugin's `stdout`.

**Request Envelope (core → plugin):**
The gateway sends a JSON object containing information relevant to the command being executed:
```json
{
  "protocol": 1,
  "job_id": "uuid-of-the-job",
  "command": "poll",           // or "handle", "health", "init"
  "config": {},                // Plugin-specific configuration from config.yaml
  "state": {},                 // Plugin's current dynamic state
  "event": {},                 // Only for "handle" command, the event payload
  "deadline_at": "ISO8601"      // Informational deadline
}
```

**Response Envelope (plugin → core):**
Plugins must respond with a JSON object detailing the outcome of their operation:
```json
{
  "status": "ok",               // "ok" or "error"
  "error": "human-readable message", // Present if status is "error"
  "retry": true,                // Defaults to true; set to false for permanent failures
  "events": [],                 // Array of events emitted by the plugin
  "state_updates": {},          // JSON object with updates to the plugin's state
  "logs": []                    // Optional array of log messages from the plugin
}
```

#### Plugin Commands
Plugins can support different commands, each serving a specific purpose:
-   `poll`: Used to fetch data or check external sources on a schedule (e.g., by the scheduler).
-   `handle`: Used to process inbound events, often triggered by webhooks or other plugins (via routing).
-   `health`: A diagnostic command to check the plugin's health.
-   `init`: A one-time setup command, run when a plugin is first discovered or its configuration changes.

#### Plugin Configuration and State
The Senechal Gateway distinguishes between static configuration and dynamic state for plugins:
-   **Config:** Static, read-only configuration provided in `config.yaml` (e.g., API keys, endpoints). These are passed to the plugin in the request envelope.
-   **State:** A dynamic JSON blob persisted by the gateway for each plugin. Plugins can read their current state and propose `state_updates` in their response. The gateway performs a shallow merge of these updates. This is typically used for things like OAuth tokens, last fetched timestamps, or other dynamic operational data.

### State Management
The Senechal Gateway uses a persistent state store for each plugin, distinct from its static configuration. While `config` (from `config.yaml`) provides static, read-only parameters, `state` allows plugins to maintain dynamic operational data across invocations.

#### Storage
Plugin state is stored as a single JSON blob per plugin within an SQLite database. This design choice provides a zero-ops, embedded database solution suitable for a personal integration server. The relevant table in the SQLite schema is `plugin_state`:

```sql
plugin_state (
  plugin_name TEXT PRIMARY KEY,
  state       JSON NOT NULL DEFAULT '{}',
  updated_at  TIMESTAMP
);
```

The `state` column holds the JSON data for a specific plugin identified by `plugin_name`.

#### Updates
When a plugin executes, it receives its current state in the request envelope. In its response, a plugin can include `state_updates`, which is a JSON object. The gateway then applies these `state_updates` using a **shallow merge** strategy: top-level keys in `state_updates` will replace the corresponding top-level keys in the existing state. Nested objects are not deeply merged; rather, the entire top-level key and its value are replaced.

#### Limitations
-   **Size Limit:** Each plugin's state blob is limited to **1 MB**. If a plugin attempts to update its state beyond this limit, the update will be rejected, and the job will fail.
-   **Shallow Merge:** Plugins must be aware of the shallow merge behavior. If a plugin needs to modify a nested field, it must return the entire updated top-level object in `state_updates`.
-   **Purpose:** State is intended for dynamic operational data (e.g., OAuth tokens, pagination cursors, last run timestamps), not for large datasets or complex relational information.

### Crash Recovery
The Senechal Gateway is designed for resilience and ensures "at-least-once" job execution semantics, even in the face of unexpected shutdowns or crashes. This is achieved through a robust crash recovery mechanism that activates automatically during startup.

#### Recovery Process
Upon startup, the gateway performs the following steps:
1.  **Acquire Lock:** It first acquires a single-instance lock (PID file) to ensure only one instance of the gateway is running.
2.  **Scan for Orphans:** The system scans its internal job queue for any jobs that have a `status` of `running`. These are considered "orphaned" jobs, meaning they were in progress when the gateway last shut down unexpectedly.
3.  **Job Re-evaluation:** For each orphaned job:
    *   The `attempt` count for the job is incremented.
    *   If the job's `attempt` count is still below its `max_attempts` limit, the job's `status` is reset to `queued`. This effectively re-queues the job for another attempt.
    *   If the job has already exhausted its `max_attempts`, its `status` is marked as `dead`, preventing further retries.
4.  **Logging:** Each recovered job is logged at a `WARN` level, including its `job_id`, `plugin`, `command`, and the outcome of the recovery (re-queued or marked dead).
5.  **Resume Operations:** After processing all orphaned jobs, the gateway resumes normal dispatching and scheduling operations.

This mechanism guarantees that no job is silently dropped due to a crash, upholding the "at-least-once" delivery guarantee. Plugins are expected to be idempotent or use their state to handle potential re-executions.

## 8. Monitoring & Observability
Senechal Gateway provides built-in endpoints for health monitoring and real-time event streaming, enabling effective observability of your integration workflows.

### GET /healthz
The `/healthz` endpoint provides a quick summary of the gateway's operational status. It does not require authentication, making it suitable for use by load balancers, container orchestrators (like Kubernetes), or simple monitoring scripts.

-   **Endpoint:** `GET http://localhost:8080/healthz`
-   **Response (200 OK):**
    ```json
    {
      "status": "ok",
      "uptime_seconds": 3600,
      "queue_depth": 5,
      "plugins_loaded": 12,
      "plugins_circuit_open": 0
    }
    ```

**Key Metrics:**
-   **queue_depth:** The number of jobs currently waiting to be processed. A consistently high number may indicate the need for more resources or plugin optimization.
-   **plugins_circuit_open:** The number of plugins whose circuit breakers are currently open due to repeated failures.

### GET /events (SSE)
The `/events` endpoint provides a real-time stream of system events using Server-Sent Events (SSE). This is invaluable for debugging complex workflows and monitoring the gateway's activity as it happens.

-   **Endpoint:** `GET http://localhost:8080/events`
-   **Authentication:** Requires a Bearer token with `events:ro` or `*` scope.
-   **Format:** `text/event-stream`

**Streaming with curl:**
```bash
TOKEN="your_token"
curl -N -H "Authorization: Bearer $TOKEN" http://localhost:8080/events
```

**Example Event Stream:**
```text
id: 101
event: job_enqueued
data: {"job_id":"...","plugin":"echo","command":"poll"}

id: 102
event: job_started
data: {"job_id":"...","plugin":"echo"}

id: 103
event: job_completed
data: {"job_id":"...","plugin":"echo","status":"ok"}
```

The gateway maintains a small buffer of recent events. If a client disconnects and reconnects using the `Last-Event-ID` header, the gateway will attempt to replay the missed events.

## 9. Configuration Reference
The Senechal Gateway's behavior is entirely driven by its configuration, defined in a `config.yaml` file. This file allows you to customize service-level settings and define plugin behavior.

### Example Configuration
Below is a comprehensive example `config.yaml` with explanations for each major section and field.

```yaml
# Senechal Gateway Configuration
# See SPEC.md for full reference

service:
  name: senechal-gw                # Name of the service (default: "senechal-gw")
  tick_interval: 60s               # How often the scheduler checks for due plugins (e.g., 30s, 1m). Default: 60s
  log_level: info                  # Minimum logging level (debug, info, warn, error). Default: info
  log_format: json                 # Output format for logs (json). Default: json
  dedupe_ttl: 24h                  # Time window for job deduplication (e.g., 1h, 7d). Default: 24h
  job_log_retention: 30d           # How long to retain completed job logs (e.g., 168h, 30d). Default: 30d

state:
  path: ./data/state.db            # Path to the SQLite database file for state persistence. Default: ./senechal.db

plugins_dir: ./plugins             # Directory where plugin subdirectories are located. Default: ./plugins

plugins:
  # Configuration for individual plugins
  withings:
    enabled: true                  # Whether this plugin is active. Default: true
    schedule:                      # Scheduling parameters for 'poll' command
      every: 6h                    # How often to run (e.g., 5m, hourly, daily, 2h).
      jitter: 30m                  # Random delay added to 'every' to prevent thundering herd.
      preferred_window:            # Optional: constrain execution to a time window
        start: "06:00"
        end: "22:00"
    config:                        # Plugin-specific static configuration (passed to plugin)
      client_id: ${WITHINGS_CLIENT_ID}  # Environment variable interpolation supported
      client_secret: ${WITHINGS_CLIENT_SECRET}
    retry:                         # Retry policy for failed jobs
      max_attempts: 4              # Total attempts (1 original + 3 retries). Default: 4
      backoff_base: 30s            # Base duration for exponential backoff. Default: 30s
    timeouts:                      # Custom timeouts for plugin commands
      poll: 60s                    # Default poll timeout: 60s
      handle: 120s                 # Default handle timeout: 120s
    circuit_breaker:               # Prevents hammering failing plugins
      threshold: 3                 # Consecutive failures before opening the circuit. Default: 3
      reset_after: 30m             # Time after which to attempt closing the circuit. Default: 30m
    max_outstanding_polls: 1       # Max concurrent 'poll' jobs for this plugin. Default: 1

  google-calendar:                 # Another example plugin configuration
    enabled: true
    schedule:
      every: 15m
      jitter: 3m
    config:
      credentials_file: ${GOOGLE_CREDS_PATH}
```

#### Environment Variable Interpolation
The Senechal Gateway supports environment variable interpolation within the `config.yaml` using the `${VAR_NAME}` syntax (e.g., `${API_KEY}`, `${DATABASE_PASSWORD}`). This is particularly useful for injecting sensitive information like API keys and secrets without hardcoding them directly into the configuration file. When the gateway loads the configuration, it will replace these placeholders with the values from the environment.

## 10. Plugin Development Guide
This section guides developers on how to create their own plugins for the Senechal Gateway.

### Bash Plugins
Creating a plugin using Bash is straightforward. The core idea is to read a JSON request from standard input (stdin), process it, and then print a JSON response to standard output (stdout).

#### Example: A Simple Bash Plugin

Let's create a minimal Bash plugin called `hello-world`.

1.  **Create the plugin directory and files:**
    ```bash
    mkdir -p plugins/hello-world
    touch plugins/hello-world/manifest.yaml
    touch plugins/hello-world/run.sh
    chmod +x plugins/hello-world/run.sh
    ```

2.  **`plugins/hello-world/manifest.yaml`:**
    ```yaml
    name: hello-world
    version: 1.0.0
    protocol: 1
    entrypoint: run.sh
    description: "A simple hello world plugin"
    commands: [poll]
    config_keys:
      optional: [name]
    ```

3.  **`plugins/hello-world/run.sh`:**
    ```bash
    #!/usr/bin/env bash
    # plugins/hello-world/run.sh

    set -euo pipefail

    # Read the JSON request from stdin
    REQUEST_JSON=$(cat)

    # Use 'jq' to parse values from the request (install 'jq' if you don't have it)
    # Alternatively, use 'python -c' or 'sed' for basic parsing if 'jq' is not available.
    # For simplicity, this example assumes 'jq' is present for parsing 'config.name'.
    PLUGIN_CONFIG_NAME=$(echo "$REQUEST_JSON" | jq -r '.config.name // "World"')
    JOB_ID=$(echo "$REQUEST_JSON" | jq -r '.job_id // "unknown"')

    # Generate the JSON response
    # The 'status' must be "ok" or "error"
    # 'events' is an array of events to emit
    # 'state_updates' is a JSON object to update the plugin's state
    # 'logs' is an array of log messages to capture
    RESPONSE_JSON=$(cat <<EOF
{
  "status": "ok",
  "events": [],
  "state_updates": {
    "last_run_job_id": "$JOB_ID",
    "last_greeting": "Hello, $PLUGIN_CONFIG_NAME!"
  },
  "logs": [
    {"level": "info", "message": "Hello, $PLUGIN_CONFIG_NAME! Job ID: $JOB_ID"}
  ]
}
EOF
)

    # Print the JSON response to stdout
    echo "$RESPONSE_JSON"
    ```

4.  **Configure in `config.yaml`:**
    Add the `hello-world` plugin to your `config.yaml` to enable it.
    ```yaml
    # config.yaml excerpt
    plugins:
      hello-world:
        enabled: true
        schedule:
          every: 1m
        config:
          name: Senechal User
    ```

When the Senechal Gateway runs and the `hello-world` plugin's schedule is due, it will execute `run.sh`. The script will read the incoming request, construct a greeting using the configured `name`, and output a JSON response. The gateway will then capture the log message and update the plugin's state with `last_run_job_id` and `last_greeting`.

**Important:** Ensure your Bash scripts have a [shebang line](https://en.wikipedia.org/wiki/Shebang_(Unix)) (e.g., `#!/usr/bin/env bash`) and are executable (`chmod +x`). Avoid complex background processes or interactive commands within your plugins. For parsing JSON in Bash, `jq` is highly recommended. If `jq' is not available on the execution environment, you would need to implement JSON parsing using `python -c` or `sed` (as seen in the `echo` plugin's `run.sh` script), or ensure `jq` is installed.

### Python Plugins
Developing plugins in Python is a common and powerful approach, leveraging Python's rich ecosystem. The principle remains the same as Bash plugins: read JSON from stdin, process, and write JSON to stdout.

#### Example: A Simple Python Plugin

Let's create a minimal Python plugin called `python-greet`.

1.  **Create the plugin directory and files:**
    ```bash
    mkdir -p plugins/python-greet
    touch plugins/python-greet/manifest.yaml
    touch plugins/python-greet/run.py
    chmod +x plugins/python-greet/run.py
    ```

2.  **`plugins/python-greet/manifest.yaml`:**
    ```yaml
    name: python-greet
    version: 1.0.0
    protocol: 1
    entrypoint: run.py
    description: "A simple Python greeting plugin"
    commands: [poll, handle]
    config_keys:
      optional: [greeting_word]
    ```

3.  **`plugins/python-greet/run.py`:**
    ```python
    #!/usr/bin/env python3
    # plugins/python-greet/run.py

    import sys
    import json
    import datetime

    def main():
        # Read the JSON request from stdin
        request_json_str = sys.stdin.read()
        request = json.loads(request_json_str)

        # Extract relevant information
        command = request.get("command", "poll")
        job_id = request.get("job_id", "unknown")
        config = request.get("config", {})
        state = request.get("state", {})
        event = request.get("event", {}) # Only present for 'handle' command

        greeting_word = config.get("greeting_word", "Hello")

        # --- Plugin Logic ---
        output_message = f"{greeting_word} from Python plugin! Command: {command}"
        if command == "handle" and event:
            output_message += f", Event Payload: {event.get('payload')}"

        # Prepare the response
        response = {
            "status": "ok",
            "events": [],  # Plugins can emit new events here
            "state_updates": {
                "last_run": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                "last_job_id": job_id,
                "current_greeting": greeting_word
            },
            "logs": [
                {"level": "info", "message": output_message}
            ]
        }

        # Print the JSON response to stdout
        json.dump(response, sys.stdout)
        sys.stdout.write("\n") # Ensure a newline at the end

    if __name__ == "__main__":
        main()
    ```

4.  **Configure in `config.yaml`:**
    Add the `python-greet` plugin to your `config.yaml`:
    ```yaml
    # config.yaml excerpt
    plugins:
      python-greet:
        enabled: true
        schedule:
          every: 1m
        config:
          greeting_word: "Greetings"
    ```

When the Senechal Gateway executes the `python-greet` plugin, `run.py` will receive the request, process it using Python's `json` module, and return a structured JSON response.

**Important:**
-   Ensure your Python scripts have a shebang line (e.g., `#!/usr/bin/env python3`) and are executable (`chmod +x`).
-   Use `sys.stdin.read()` to get the entire input and `json.loads()` to parse it.
-   Use `json.dump(response, sys.stdout)` to write the response. It's good practice to add `sys.stdout.write("\n")` to ensure the output is properly terminated.
-   If your Python plugin requires external libraries, you should manage them within the plugin's directory (e.g., using a `venv` and installing dependencies locally) or ensure they are available in the execution environment. The Senechal Gateway itself does not manage plugin-specific Python environments.

### Manifest Command Metadata
To take full advantage of Senechal Gateway's authorization system, you should provide metadata for the commands your plugin supports in its `manifest.yaml`.

#### Command Formats
The `commands` field in `manifest.yaml` supports two formats:

1.  **Simple Array (Legacy):**
    ```yaml
    commands: [poll, health]
    ```
    *Behavior:* `health` is automatically assigned `type: read`, all others are `type: write`.

2.  **Object Array (Recommended):**
    ```yaml
    commands:
      - name: poll
        type: write
      - name: health
        type: read
      - name: status
        type: read
    ```

#### Read vs Write Commands
-   **read:** Commands that only return information and have no side effects. Can be executed by tokens with `plugin:ro` scope.
-   **write:** Commands that fetch data, update external systems, or change local state. Require `plugin:rw` scope.

#### Why use explicit types?
Explicitly marking commands as `read` allows you to create highly restricted "Monitoring Tokens" that can check plugin health and status but cannot trigger data synchronization or other potentially expensive or sensitive actions.

## 11. Operations and Troubleshooting
This section covers how to operate the Senechal Gateway in production or development environments, including monitoring, understanding logs, and common troubleshooting scenarios.

### Logging
The Senechal Gateway core emits structured logs in JSON format to standard output (stdout). These logs are designed to be easily parseable by log aggregation systems.

#### Core Logs
Core logs include fields such as:
-   `timestamp`: ISO8601 formatted timestamp of the log entry.
-   `level`: The log level (e.g., `debug`, `info`, `warn`, `error`). Configurable via `service.log_level`.
-   `component`: The internal component emitting the log (e.g., `scheduler`, `dispatch`).
-   `plugin`: The name of the plugin involved (if applicable).
-   `job_id`: The ID of the job involved (if applicable).
-   `message`: A human-readable description of the event.

Example:
```json
{"level":"info","component":"scheduler","msg":"Senechal Gateway starting...","time":"2026-02-09T12:00:00Z"}
{"level":"info","plugin":"echo","schedule":"5m","msg":"scheduling plugin","time":"2026-02-09T12:00:01Z"}
```

#### Plugin Logs
Plugins can also emit logs as part of their response envelope. These logs are captured by the gateway and stored with the job record.
```json
"logs": [
  {"level": "info", "message": "Hello, Senechal User! Job ID: uuid-of-the-job"}
]
```
Additionally, anything written by a plugin to `stderr` is captured, capped at 64 KB, and logged at `WARN` level to the core log stream, along with the job details. Plugin `stdout` is reserved exclusively for the protocol response; any non-JSON output on `stdout` is treated as a protocol error, causing the job to fail.

### Command Line Interface (CLI)
The `senechal-gw` executable provides several commands for interacting with and monitoring the gateway:

-   `senechal-gw start`: Runs the service in the foreground.
-   `senechal-gw run <plugin>`: Manually runs a specific plugin's `poll` command once.
-   `senechal-gw status`: Shows the state of discovered plugins, queue depth, and recent runs.
-   `senechal-gw reload`: Sends a `SIGHUP` signal to the running process to reload the configuration without a full restart.
-   `senechal-gw reset <plugin>`: Resets the circuit breaker for a specified plugin.
-   `senechal-gw plugins`: Lists all discovered plugins and their current status.
-   `senechal-gw logs [plugin]`: Tails structured logs, optionally filtered by plugin.
-   `senechal-gw queue`: Shows pending and active jobs in the work queue.

### Troubleshooting
-   **Plugin Not Running:**
    *   Check `config.yaml`: Ensure the plugin is `enabled: true` and its `schedule` is correctly defined.
    *   Examine gateway logs for `WARN` or `ERROR` messages related to plugin discovery, manifest parsing, or scheduling.
    *   Verify the plugin's `manifest.yaml` (especially `protocol` version and `entrypoint`).
    *   Ensure the plugin entrypoint script is executable (`chmod +x`).
-   **Job Failures:**
    *   Check the `stderr` output captured in the logs for the failed job. This often contains specific error messages from the plugin.
    *   Review the plugin's internal logic.
    *   Ensure the plugin is returning a valid JSON response on `stdout`.
    *   Check for timeouts (default: 60s for `poll`, 120s for `handle`). Increase if necessary in `config.yaml`.
-   **State Updates Not Working:**
    *   Ensure `state_updates` in the plugin's response are valid JSON.
    *   Check if the plugin's state blob is exceeding the 1MB size limit.
    *   Remember that state updates are shallow merges; nested objects are replaced, not deeply merged.
-   **"Thundering Herd" on Schedules:**
    *   Adjust the `jitter` value in `config.yaml` for scheduled plugins to introduce more randomization.
-   **Gateway Not Starting:**
    *   Check for PID lock errors (only one instance can run at a time). Ensure previous instances are fully shut down.
    *   Validate your `config.yaml` for syntax errors. The gateway will usually report these at startup.
    *   Verify SQLite database path is accessible and writable.