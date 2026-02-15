# Plugin Development Guide

Ductile is built on a **spawn-per-command** model. Any executable that can read JSON from `stdin` and write JSON to `stdout` can serve as a Ductile plugin (or "Skill").

---

## 1. The Lifecycle

When a job is triggered (via scheduler, API, or webhook):
1.  Ductile forks the plugin entrypoint.
2.  The core writes a **Request Envelope** to the plugin's `stdin`.
3.  The plugin processes the command and writes a **Response Envelope** to `stdout`.
4.  Ductile captures `stderr` for logging and kills the process if it exceeds the timeout.

---

## 2. Protocol v2

### Request Envelope (Core → Plugin)
```json
{
  "protocol": 2,
  "job_id": "uuid",
  "command": "poll | handle | health | init",
  "config": {},
  "state": {},
  "context": {},
  "workspace_dir": "/path/to/workspace",
  "event": {},
  "deadline_at": "ISO8601"
}
```
-   `context`: Shared metadata ("Baggage") carried across the pipeline chain.
-   `workspace_dir`: A dedicated local directory for this job's artifacts.

### Response Envelope (Plugin → Core)
```json
{
  "status": "ok | error",
  "error": "message",
  "retry": true,
  "events": [],
  "state_updates": {},
  "logs": []
}
```
-   `state_updates`: Top-level keys here are shallow-merged into the plugin's persistent state.
-   `events`: An array of `{ "type": "...", "payload": {} }` to trigger downstream pipelines.

---

## 3. Bash Example

```bash
#!/usr/bin/env bash
set -euo pipefail

# Read the request
REQUEST=$(cat)

# Extract fields using jq
COMMAND=$(echo "$REQUEST" | jq -r '.command')
MESSAGE=$(echo "$REQUEST" | jq -r '.config.message // "Hello"')

# Write a log file to the workspace
WORKSPACE=$(echo "$REQUEST" | jq -r '.workspace_dir')
echo "$MESSAGE" > "$WORKSPACE/output.txt"

# Respond
cat <<EOF
{
  "status": "ok",
  "logs": [{"level": "info", "message": "Command $COMMAND executed successfully"}]
}
EOF
```

---

## 4. Python Example

```python
import sys, json, os

def main():
    # Read request
    req = json.load(sys.stdin)
    command = req.get("command")
    config = req.get("config", {})
    
    # Process
    if command == "poll":
        # ... logic ...
        pass
        
    # Build response
    resp = {
        "status": "ok",
        "state_updates": {"last_seen": "now"},
        "logs": [{"level": "info", "message": "Python plugin active"}]
    }
    
    # Write response
    json.dump(resp, sys.stdout)

if __name__ == "__main__":
    main()
```

---

## 5. Security & Isolation

-   **Allowed Paths:** Plugins should only read/write within their provided `workspace_dir` or explicitly configured paths.
-   **Execution:** Plugins run as the same OS user as the gateway. Use filesystem permissions to limit their scope.
-   **Trust:** Ductile will refuse to load plugins with world-writable directories or path traversal attempts in their entrypoints.
