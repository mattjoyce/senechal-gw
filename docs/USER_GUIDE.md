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


## Table of Contents
1.  [Introduction](#introduction)
2.  [Getting Started](#getting-started)
    *   [Installation](#installation)
    *   [Basic Usage](#basic-usage)
3.  [Using the API](#using-the-api)
    *   [API Endpoints Introduction](#api-endpoints-introduction)
    *   [API Configuration](#api-configuration)
    *   [POST /trigger/{plugin}/{command}](#post-triggerplugincommand)
    *   [GET /job/{job_id}](#get-jobjob_id)
    *   [API Use Cases](#api-use-cases)
    *   [API Security Notes](#api-security-notes)
4.  [Core Concepts](#core-concepts)
    *   [Scheduler](#scheduler)
    *   [Plugins](#plugins)
    *   [State Management](#state-management)
    *   [Crash Recovery](#crash-recovery)
5.  [Configuration Reference](#configuration-reference)
    *   [Example Configuration](#example-configuration)
6.  [Plugin Development Guide](#plugin-development-guide)
    *   [Bash Plugins](#bash-plugins)
    *   [Python Plugins](#python-plugins)
7.  [Operations and Troubleshooting](#operations-and-troubleshooting)

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

## 3. Using the API
The Senechal Gateway provides a simple HTTP API to programmatically trigger plugins and retrieve job results. This is particularly useful for external systems, such as LLM agents or custom scripts, that need to interact with the gateway on-demand rather than relying solely on scheduled tasks.

### API Configuration
To enable and configure the API, add an `api` section to your `config.yaml`:

```yaml
# config.yaml excerpt
api:
  enabled: true                    # Set to true to enable the HTTP API
  listen: "localhost:8080"         # Address and port for the API listener
  auth:
    api_key: ${API_KEY}            # Bearer token for authentication (use environment variable for secret)
```

The `api.auth.api_key` should always be provided via an environment variable (e.g., `${API_KEY}`) for security best practices.

### POST /trigger/{plugin}/{command}
This endpoint allows you to enqueue a job for a specific plugin and command. The API call returns immediately with a job ID, and the plugin execution happens asynchronously.

-   **Purpose:** Enqueue jobs on-demand.
-   **Authentication:** Requires a Bearer token in the `Authorization` header, matching the configured `api.auth.api_key`.
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
    *   `400 Bad Request`: Plugin not found, command not supported, or invalid request body.
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
-   **Authentication:** Requires a Bearer token in the `Authorization` header.
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

### API Security Notes
-   **API Key Required:** All API requests *must* include a valid API key for authentication.
-   **Environment Variables:** Always store your API key in an environment variable (e.g., `API_KEY`) and use `${API_KEY}` in your `config.yaml` to prevent sensitive information from being committed to source control.
-   **Localhost Only by Default:** The `api.listen` address defaults to `localhost:8080`. For external access, it is highly recommended to place the Senechal Gateway behind a reverse proxy (e.g., Nginx, Caddy) that handles TLS termination, rate limiting, and additional security measures.

