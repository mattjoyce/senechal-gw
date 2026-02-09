---
id: 30
status: todo
priority: Normal
blocked_by: []
assignee: "@gemini"
tags: [sprint-2, documentation]
---

# User Guide Documentation

Create comprehensive user guide documenting the current MVP functionality. Helps users understand, configure, and extend Senechal Gateway.

## Acceptance Criteria

- User guide created at `docs/USER_GUIDE.md`
- Clear, well-structured, beginner-friendly
- Covers all MVP features (scheduler, plugins, state, crash recovery)
- Step-by-step setup and usage instructions
- Configuration examples for common scenarios
- Plugin development guide with examples
- Troubleshooting section for common issues
- 2000-3000 words, properly formatted Markdown

## Document Structure

### 1. Introduction (200 words)
- **What is Senechal Gateway?**
  - Personal-scale automation gateway
  - Event-driven plugin orchestration
  - State persistence, crash recovery

- **Use Cases**
  - Automated data collection (APIs, sensors)
  - Health monitoring (services, websites)
  - Data sync between systems
  - Personal automation (fitness, finance, social)
  - IoT device integration
  - Backup orchestration

- **Architecture Overview**
  - Scheduler → Queue → Dispatcher → Plugins
  - SQLite state persistence
  - Protocol v1 JSON I/O over stdin/stdout
  - Serial execution (one job at a time)

### 2. Getting Started (300 words)
- **Prerequisites**
  - Go 1.21+ installed
  - SQLite (included with modernc.org/sqlite)
  - Basic command line knowledge

- **Installation**
  ```bash
  git clone https://github.com/mattjoyce/senechal-gw
  cd senechal-gw
  go build -o senechal-gw ./cmd/senechal-gw
  ```

- **First Run**
  ```bash
  ./senechal-gw start --config config.yaml
  # Watch logs, press Ctrl+C to stop
  ```

- **Verify It Works**
  - Echo plugin should execute every 5 minutes
  - Check logs for "Enqueued poll job"
  - Query SQLite for plugin state
  ```bash
  sqlite3 senechal.db "SELECT * FROM plugin_state;"
  ```

### 3. Configuration Reference (500 words)
- **Service Settings**
  - `service.tick_interval` - Scheduler heartbeat (default: 60s)
  - `service.log_level` - debug, info, warn, error (default: info)
  - `service.job_log_retention` - How long to keep job history
  - `state.path` - SQLite database location

- **Plugin Configuration**
  - `enabled` - Enable/disable plugin
  - `schedule.every` - Poll interval (5m, hourly, daily, etc.)
  - `schedule.jitter` - Randomization to avoid thundering herd
  - `timeout` - Max execution time (default: 60s)
  - `max_attempts` - Retry limit (default: 3)
  - `config` - Plugin-specific settings

- **Example: GitHub Repo Monitor**
  ```yaml
  plugins:
    github_monitor:
      enabled: true
      schedule:
        every: hourly
        jitter: 5m
      timeout: 30s
      max_attempts: 3
      config:
        token: ${GITHUB_TOKEN}
        repos: ["user/repo1", "user/repo2"]
  ```

### 4. Using Plugins (400 words)
- **Echo Plugin Walkthrough**
  - What it does (test plugin, returns timestamp)
  - How to configure
  - Where to find output (logs + state)

- **Check Plugin State**
  ```bash
  sqlite3 senechal.db "SELECT plugin, state FROM plugin_state;"
  ```

- **View Job History**
  ```bash
  sqlite3 senechal.db \
    "SELECT job_id, plugin, status, completed_at FROM job_log
     ORDER BY completed_at DESC LIMIT 10;"
  ```

- **Monitor Logs**
  - Structured JSON logs to stdout
  - Filter by plugin: `jq 'select(.plugin=="echo")'`
  - Filter by level: `jq 'select(.level=="ERROR")'`

### 5. Writing Plugins (600 words)
- **Protocol v1 Overview**
  - Request envelope (JSON via stdin)
  - Response envelope (JSON via stdout)
  - State management (OAuth tokens, cursors)

- **Request Format**
  ```json
  {
    "protocol": 1,
    "job_id": "uuid",
    "command": "poll",
    "config": {...},
    "state": {...},
    "deadline_at": "ISO8601"
  }
  ```

- **Response Format**
  ```json
  {
    "status": "ok",
    "state_updates": {"last_run": "timestamp"},
    "logs": [{"level": "info", "message": "..."}]
  }
  ```

- **Example: Bash Plugin**
  ```bash
  #!/bin/bash
  # Read request from stdin
  REQUEST=$(cat)

  # Extract config (using jq)
  MESSAGE=$(echo "$REQUEST" | jq -r '.config.message')

  # Do work
  TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

  # Write response to stdout
  jq -n --arg ts "$TIMESTAMP" '{
    status: "ok",
    state_updates: {last_run: $ts},
    logs: [{level: "info", message: "Plugin executed"}]
  }'
  ```

- **Example: Python Plugin**
  ```python
  #!/usr/bin/env python3
  import json
  import sys
  from datetime import datetime

  # Read request
  request = json.load(sys.stdin)

  # Do work
  result = {"timestamp": datetime.utcnow().isoformat()}

  # Write response
  response = {
      "status": "ok",
      "state_updates": {"last_run": result["timestamp"]},
      "logs": [{"level": "info", "message": "Plugin executed"}]
  }
  json.dump(response, sys.stdout)
  ```

- **Manifest Requirements**
  - `manifest.yaml` in plugin directory
  - Required fields: name, protocol, entrypoint, commands
  - Optional: version, description, config_keys

- **Testing Plugins Standalone**
  ```bash
  echo '{"protocol":1,"job_id":"test","command":"poll","config":{},"state":{}}' \
    | ./plugins/my_plugin/run.sh | jq
  ```

### 6. Operations (300 words)
- **Starting the Service**
  ```bash
  ./senechal-gw start --config config.yaml
  ```

- **Graceful Shutdown**
  - Press Ctrl+C (SIGINT)
  - Or: `kill -TERM $(cat senechal.pid)`
  - Waits for running jobs to complete

- **PID Lock Behavior**
  - Only one instance runs at a time
  - Lock file: `{state_dir}/senechal.pid`
  - If locked: another instance is running

- **Database Management**
  - Backup: `cp senechal.db senechal.db.backup`
  - Prune old logs: automatic via `job_log_retention`
  - Manual prune: `DELETE FROM job_log WHERE completed_at < ...`

- **Log Analysis**
  - Tail logs: `./senechal-gw start | tee senechal.log`
  - Parse JSON: `cat senechal.log | jq 'select(.level=="ERROR")'`
  - Count jobs: `jq -s 'group_by(.plugin) | map({plugin:.[0].plugin, count:length})'`

### 7. Troubleshooting (300 words)
- **"Failed to acquire PID lock"**
  - Another instance is running
  - Check: `ps aux | grep senechal`
  - Kill stale process or delete `senechal.pid`

- **Plugin timeout**
  - Plugin took longer than configured timeout
  - Increase `timeout` in config
  - Check plugin logs for what's slow

- **Database locked**
  - SQLite file permissions issue
  - Check: `ls -l senechal.db`
  - Fix: `chmod 644 senechal.db`

- **Plugin not discovered**
  - Check plugin directory path in config
  - Verify `manifest.yaml` exists
  - Check entrypoint is executable: `chmod +x run.sh`

- **Enable debug logging**
  ```yaml
  service:
    log_level: debug
  ```

### 8. Advanced Topics (300 words)
- **Crash Recovery**
  - Orphaned jobs (status=running) on startup
  - Re-queued if under max_attempts
  - Marked dead if attempts exhausted

- **Job Deduplication**
  - Configured via `dedupe_key` (not enforced in MVP)
  - Prevents duplicate work in `dedupe_ttl` window

- **Schedule Jitter**
  - Adds randomness to schedule (0 to jitter value)
  - Prevents thundering herd (multiple plugins at once)
  - Example: hourly + 5m jitter = 60-65 min interval

- **SQLite Schema**
  - `job_queue` - Active jobs (queued/running)
  - `job_log` - Completed job history
  - `plugin_state` - Per-plugin state blobs

## Branch

`gemini/user-guide`

## Deliverable

Single comprehensive Markdown file at `docs/USER_GUIDE.md`

## Style Guidelines

- Clear, beginner-friendly language
- Use code blocks for examples (with syntax highlighting)
- Use headers for structure (##, ###)
- Use bullet points and numbered lists
- Include realistic examples (not just "foo", "bar")
- Keep paragraphs short (3-4 sentences)
- Use bold for **emphasis** on important points

## Verification

- Read through entire guide
- Verify all code examples are syntactically correct
- Test example commands actually work
- Check for typos and formatting issues
- Ensure all MVP features are documented

## Narrative

- 2026-02-09: PR #9 submitted. Review feedback: Guide is comprehensive (3689 words, 6 sections), well-structured, and covers MVP features effectively. However, configuration section includes unimplemented Sprint 3 features (webhooks, routes) that need to be removed. User instructed to focus on pre-Sprint 2 code (MVP only). Request revision to remove webhooks/routes config examples and keep only: service settings, state path, plugins_dir, and plugins.* configuration (schedule, config, timeouts, retry). All other sections (intro, getting started, core concepts, plugin development, troubleshooting) are excellent. (by @claude)