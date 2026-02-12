# Senechal Gateway User Guide

## Table of Contents
1.  [Introduction](#introduction)
2.  [Getting Started](#getting-started)
    *   [Installation](#installation)
    *   [Basic Usage](#basic-usage)
3.  [Core Concepts](#core-concepts)
    *   [Scheduler](#scheduler)
    *   [Plugins](#plugins)
    *   [State Management](#state-management)
    *   [Crash Recovery](#crash-recovery)
4.  [Configuration Reference](#configuration-reference)
    *   [Example Configuration](#example-configuration)
5.  [Plugin Development Guide](#plugin-development-guide)
    *   [Bash Plugins](#bash-plugins)
    *   [Python Plugins](#python-plugins)
6.  [Operations and Troubleshooting](#operations-and-troubleshooting)

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
    ./senechal-gw system start
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

---

### CLI Principles

To ensure predictability and safety, all Senechal CLI commands follow these standards:

- **NOUN VERB Hierarchy:** Commands are organized by resource (e.g., `job inspect`, `config lock`).
- **Verbosity:** Use `-v` or `--verbose` to see internal logic and state transitions.
- **Dry Run:** Use `--dry-run` for any mutation to preview changes without committing them.
- **Machine-Readability:** Use `--json` to get structured data for scripts or LLMs.


## 3. Core Concepts

### Scheduler
The Senechal Gateway includes a built-in scheduler responsible for orchestrating the periodic execution of configured plugins. It operates on a "tick loop" that runs at a configurable interval (defaulting to `service.tick_interval`, typically 60 seconds).

For each enabled plugin with a defined `schedule`, the scheduler calculates its next execution time using a "fuzzy interval" approach. This approach incorporates a fixed jitter to the scheduled interval. The jitter (e.g., `jitter: 30s`) introduces a random delay within a specified window, preventing all scheduled plugins from attempting to run simultaneously, thus avoiding a "thundering herd" problem and distributing load more evenly.

When a plugin's scheduled time arrives, the scheduler enqueues a `poll` job for that plugin into the system's work queue. This `poll` job initiates the plugin's execution flow. The scheduler also ensures "at-least-once" job execution semantics by utilizing deduplication keys.

The `schedule.every` field supports various named intervals such as `5m`, `15m`, `30m`, `hourly`, `2h`, `6h`, `daily`, `weekly`, and `monthly`.

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

## 4. Configuration Reference
The Senechal Gateway's behavior is entirely driven by its configuration, defined in a `config.yaml` file. This file allows you to customize service-level settings, define plugin behavior, and set up advanced features like webhooks and routing.

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

webhooks:
  listen: 127.0.0.1:8081           # Address and port for the webhook listener.
  endpoints:                       # List of webhook endpoints
    - path: /hook/github           # URL path for this endpoint
      plugin: github-handler       # Plugin to invoke with the 'handle' command
      secret: ${GITHUB_WEBHOOK_SECRET} # Secret for HMAC-SHA256 verification
      signature_header: X-Hub-Signature-256 # HTTP header containing the signature
      max_body_size: 1MB           # Max request body size.

routes:
  # Define how events emitted by plugins are routed to other plugins
  - from: withings                 # Source plugin emitting the event
    event_type: new_health_data    # Type of event to match
    to: health-analyzer            # Target plugin to receive the event (via 'handle' command)

  - from: health-analyzer
    event_type: alert
    to: notify
```

#### Environment Variable Interpolation
The Senechal Gateway supports environment variable interpolation within the `config.yaml` using the `${VAR_NAME}` syntax (e.g., `${GITHUB_WEBHOOK_SECRET}`). This is particularly useful for injecting sensitive information like API keys and secrets without hardcoding them directly into the configuration file. When the gateway loads the configuration, it will replace these placeholders with the values from the environment.


## 5. Plugin Development Guide
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

**Important:** Ensure your Bash scripts have a [shebang line](https://en.wikipedia.org/wiki/Shebang_(Unix)) (e.g., `#!/usr/bin/env bash`) and are executable (`chmod +x`). Avoid complex background processes or interactive commands within your plugins. For parsing JSON in Bash, `jq` is highly recommended. If `jq` is not available on the execution environment, you would need to implement JSON parsing using `python -c` or `sed` (as seen in the `echo` plugin's `run.sh` script), or ensure `jq` is installed.

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

## 6. Operations and Troubleshooting
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
The `senechal-gw` executable provides a structured command hierarchy for interacting with and monitoring the gateway:

-   `senechal-gw system start`: Runs the service in the foreground.
-   `senechal-gw config lock`: Authorizes current configuration by updating integrity hashes.
-   `senechal-gw config check`: Validates configuration syntax, policy, and integrity.
-   `senechal-gw config show [entity]`: Displays the full or partial configuration (e.g., `config show plugin:echo`).
-   `senechal-gw config get <path>`: Retrieves a specific value using dot-notation (e.g., `config get service.name`).
-   `senechal-gw config set <path>=<value>`: Modifies a configuration value. Requires `--dry-run` or `--apply`.
-   `senechal-gw job inspect <id>`: Shows the full lineage, baggage, and artifacts for a job.
-   `senechal-gw system status`: Shows the state of discovered plugins, queue depth, and health (planned).
-   `senechal-gw system reload`: Sends a `SIGHUP` signal to reload configuration without restart (planned).
-   `senechal-gw plugin list`: Lists all discovered plugins and their current status (planned).
-   `senechal-gw queue status`: Shows pending and active jobs in the work queue (planned).

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
