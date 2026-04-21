#!/usr/bin/env bash
set -euo pipefail

request="$(cat)"

command_val="$(printf '%s' "$request" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("command","poll") or "poll")')"
message="$(printf '%s' "$request" | python3 -c 'import json,sys; d=json.load(sys.stdin); cfg=d.get("config") or {}; print(cfg.get("message","fixture echo") or "fixture echo")')"
timestamp="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

python3 - "$command_val" "$message" "$timestamp" <<'PY'
import json, sys
command_val = sys.argv[1]
message = sys.argv[2]
timestamp = sys.argv[3]
if command_val not in {"poll", "handle", "health"}:
    json.dump({"status": "error", "error": f"unknown command: {command_val}", "retry": False}, sys.stdout)
    raise SystemExit(0)
json.dump({
    "status": "ok",
    "result": f"{message} at {timestamp}",
    "events": [],
    "logs": [{"level": "info", "message": f"{message} at {timestamp}"}],
}, sys.stdout)
PY
