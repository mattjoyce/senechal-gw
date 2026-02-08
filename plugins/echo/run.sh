#!/usr/bin/env bash
# Echo plugin - Protocol v1 test plugin
# Reads JSON from stdin, writes JSON to stdout

set -euo pipefail

# Read request from stdin
request=$(cat)

# Parse request fields (using jq if available, otherwise basic parsing)
if command -v jq &> /dev/null; then
    command=$(echo "$request" | jq -r '.command')
    job_id=$(echo "$request" | jq -r '.job_id')
    message=$(echo "$request" | jq -r '.config.message // "Echo plugin running"')
else
    # Fallback: basic parsing without jq
    command=$(echo "$request" | grep -o '"command":"[^"]*"' | cut -d':' -f2 | tr -d '"')
    job_id=$(echo "$request" | grep -o '"job_id":"[^"]*"' | cut -d':' -f2 | tr -d '"')
    message="Echo plugin running"
fi

# Current timestamp
timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Handle commands
case "$command" in
    poll|health)
        # Return success with state update
        cat <<EOF
{
  "status": "ok",
  "state_updates": {
    "last_run": "$timestamp",
    "job_id": "$job_id"
  },
  "logs": [
    {"level": "info", "message": "$message at $timestamp"}
  ]
}
EOF
        ;;
    *)
        # Unknown command
        cat <<EOF
{
  "status": "error",
  "error": "Unknown command: $command",
  "retry": false
}
EOF
        exit 1
        ;;
esac
