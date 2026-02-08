#!/usr/bin/env bash
# Echo plugin - protocol v1 test plugin.
# Reads JSON request from stdin, writes JSON response to stdout.
#
# Supports optional config for testing paths:
# - config.message: string included in logs
# - config.mode: "" | "error" | "hang" | "protocol_error"

set -euo pipefail

request="$(cat)"

have_jq=false
command -v jq >/dev/null 2>&1 && have_jq=true

have_py=false
command -v python3 >/dev/null 2>&1 && have_py=true

get_command() {
  if $have_jq; then
    printf '%s' "$request" | jq -r '.command // "poll"' 2>/dev/null || printf '%s' "poll"
    return 0
  fi
  if $have_py; then
    printf '%s' "$request" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("command","poll") or "poll")' 2>/dev/null || printf '%s' "poll"
    return 0
  fi
  printf '%s' "$request" | sed -n 's/.*"command"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1 || true
}

get_job_id() {
  if $have_jq; then
    printf '%s' "$request" | jq -r '.job_id // ""' 2>/dev/null || true
    return 0
  fi
  if $have_py; then
    printf '%s' "$request" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("job_id","") or "")' 2>/dev/null || true
    return 0
  fi
  printf '%s' "$request" | sed -n 's/.*"job_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1 || true
}

get_config_message() {
  if $have_jq; then
    printf '%s' "$request" | jq -r '.config.message // "echo plugin ran"' 2>/dev/null || printf '%s' "echo plugin ran"
    return 0
  fi
  if $have_py; then
    printf '%s' "$request" | python3 -c 'import json,sys; d=json.load(sys.stdin); cfg=d.get("config") or {}; print(cfg.get("message","echo plugin ran") or "echo plugin ran")' 2>/dev/null || printf '%s' "echo plugin ran"
    return 0
  fi
  printf '%s' "echo plugin ran"
}

get_config_mode() {
  if $have_jq; then
    printf '%s' "$request" | jq -r '.config.mode // ""' 2>/dev/null || true
    return 0
  fi
  if $have_py; then
    printf '%s' "$request" | python3 -c 'import json,sys; d=json.load(sys.stdin); cfg=d.get("config") or {}; print(cfg.get("mode","") or "")' 2>/dev/null || true
    return 0
  fi
  printf '%s' ""
}

command_val="$(get_command)"
job_id="$(get_job_id)"
message="$(get_config_message)"
mode="$(get_config_mode)"

if [[ -z "$command_val" ]]; then
  command_val="poll"
fi

timestamp="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

emit_response() {
  # args: <status> <error_message_or_empty>
  local status="$1"
  local err="${2:-}"

  if $have_jq; then
    if [[ "$status" == "ok" ]]; then
      jq -n \
        --arg ts "$timestamp" \
        --arg msg "$message" \
        --arg job "$job_id" \
        '{status:"ok", events:[], state_updates:{last_run:$ts, job_id:$job}, logs:[{level:"info", message:($msg + " at " + $ts)}] }'
      return 0
    fi

    jq -n \
      --arg err "$err" \
      '{status:"error", error:$err, retry:false, logs:[{level:"error", message:$err}] }'
    return 0
  fi

  if $have_py; then
    python3 - "$status" "$err" "$timestamp" "$message" "$job_id" <<'PY'
import json, sys
status = sys.argv[1]
err = sys.argv[2]
ts = sys.argv[3]
msg = sys.argv[4]
job = sys.argv[5]
if status == "ok":
  out = {
    "status": "ok",
    "events": [],
    "state_updates": {"last_run": ts, "job_id": job},
    "logs": [{"level": "info", "message": f"{msg} at {ts}"}],
  }
else:
  out = {
    "status": "error",
    "error": err or "unknown error",
    "retry": False,
    "logs": [{"level": "error", "message": err or "unknown error"}],
  }
json.dump(out, sys.stdout)
PY
    return 0
  fi

  # Last-resort JSON output with minimal escaping.
  local esc_msg="${message//\\/\\\\}"
  esc_msg="${esc_msg//\"/\\\"}"
  local esc_err="${err//\\/\\\\}"
  esc_err="${esc_err//\"/\\\"}"
  if [[ "$status" == "ok" ]]; then
    printf '{"status":"ok","events":[],"state_updates":{"last_run":"%s","job_id":"%s"},"logs":[{"level":"info","message":"%s at %s"}]}\n' \
      "$timestamp" "$job_id" "$esc_msg" "$timestamp"
  else
    printf '{"status":"error","error":"%s","retry":false,"logs":[{"level":"error","message":"%s"}]}\n' \
      "$esc_err" "$esc_err"
  fi
}

case "$mode" in
  hang)
    sleep 999
    ;;
  protocol_error)
    printf '{"status": "ok", "state_updates": {"last_run": "%s"}' "$timestamp"
    exit 0
    ;;
esac

case "$command_val" in
  poll|health)
    if [[ "$mode" == "error" ]]; then
      emit_response "error" "echo plugin forced error (mode=error)"
      exit 0
    fi
    emit_response "ok" ""
    exit 0
    ;;
  *)
    emit_response "error" "unknown command: $command_val"
    exit 0
    ;;
esac
