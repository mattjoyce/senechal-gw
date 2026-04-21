#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:?}"
FIXTURE_NAME="${FIXTURE_NAME:?}"
ARTIFACT_ROOT="${ARTIFACT_ROOT:?}"
# shellcheck source=/dev/null
source "$ROOT_DIR/scripts/test-docker-lib"
fixture_init

CONFIG_DIR="$FIXTURE_DIR/config"
STATE_DIR="$CONFIG_DIR/state"
DB_PATH="$STATE_DIR/ductile.db"
PID=""
rm -rf "$STATE_DIR"
mkdir -p "$STATE_DIR"
SCENARIO_LOG="$ARTIFACT_DIR/scenario.log"
exec > >(tee "$SCENARIO_LOG") 2>&1

cleanup() {
  fixture_capture_tree "$CONFIG_DIR" config
  fixture_capture_tree "$STATE_DIR" state-dir
  fixture_capture_file "$DB_PATH" state.db
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

fixture_log "starting ductile process"
"$ROOT_DIR/ductile" system start --config "$CONFIG_DIR" >"$ARTIFACT_DIR/ductile.log" 2>&1 &
PID=$!

ready=0
for _ in $(seq 1 40); do
  if curl -fsS http://127.0.0.1:18481/healthz >"$ARTIFACT_DIR/healthz.json" 2>/dev/null; then
    ready=1
    break
  fi
  sleep 0.25
done
if [[ "$ready" -ne 1 ]]; then
  fixture_fail "health endpoint did not become ready"
fi

fixture_log "triggering root job that should fire hook pipeline"
STATUS_CODE=$(curl -sS -o "$ARTIFACT_DIR/trigger-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18481/plugin/root_echo/poll \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"message":"hook route fixture"}}')
if [[ "$STATUS_CODE" != "202" ]]; then
  fixture_fail "expected 202 for root plugin trigger, got $STATUS_CODE"
fi

ROOT_JOB_ID=$(jq -r '.job_id' "$ARTIFACT_DIR/trigger-response.json")
if [[ -z "$ROOT_JOB_ID" || "$ROOT_JOB_ID" == "null" ]]; then
  fixture_fail "root trigger returned no job_id"
fi

fixture_log "waiting for root and hook jobs to settle"
for _ in $(seq 1 60); do
  if [[ -f "$DB_PATH" ]]; then
    TOTAL=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_log;" 2>/dev/null || echo "0")
    if [[ "$TOTAL" -ge 2 ]]; then
      break
    fi
  fi
  sleep 0.25
done

TOTAL=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_log;")
printf '%s\n' "$TOTAL" >"$ARTIFACT_DIR/job-log-count.txt"
if [[ "$TOTAL" != "2" ]]; then
  fixture_fail "expected exactly 2 completed jobs (root + hook), found $TOTAL"
fi

ROOT_STATUS=$(sqlite3 "$DB_PATH" "SELECT status FROM job_log WHERE plugin = 'root_echo' AND command = 'poll' ORDER BY completed_at DESC LIMIT 1;")
HOOK_STATUS=$(sqlite3 "$DB_PATH" "SELECT status FROM job_log WHERE plugin = 'hook_echo' AND command = 'handle' ORDER BY completed_at DESC LIMIT 1;")
printf '%s\n' "$ROOT_STATUS" >"$ARTIFACT_DIR/root-status.txt"
printf '%s\n' "$HOOK_STATUS" >"$ARTIFACT_DIR/hook-status.txt"
if [[ "$ROOT_STATUS" != "succeeded" ]]; then
  fixture_fail "expected root job to succeed, got $ROOT_STATUS"
fi
if [[ "$HOOK_STATUS" != "succeeded" ]]; then
  fixture_fail "expected hook job to succeed, got $HOOK_STATUS"
fi

EVENT_CONTEXT_COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM event_context;" 2>/dev/null || echo "0")
printf '%s\n' "$EVENT_CONTEXT_COUNT" >"$ARTIFACT_DIR/event-context-count.txt"
if [[ "$EVENT_CONTEXT_COUNT" != "0" ]]; then
  fixture_fail "expected hook dispatch to remain root-level with 0 event_context rows, found $EVENT_CONTEXT_COUNT"
fi

HOOK_PAYLOAD_FILE="$ARTIFACT_DIR/hook-payload.json"
sqlite3 "$DB_PATH" "SELECT payload FROM job_queue WHERE submitted_by = 'hook' ORDER BY created_at DESC LIMIT 1;" >"$HOOK_PAYLOAD_FILE"

HOOK_TYPE=$(jq -r '.type' "$HOOK_PAYLOAD_FILE")
HOOK_PLUGIN=$(jq -r '.payload.plugin' "$HOOK_PAYLOAD_FILE")
HOOK_COMMAND=$(jq -r '.payload.command' "$HOOK_PAYLOAD_FILE")
HOOK_STATUS_FIELD=$(jq -r '.payload.status' "$HOOK_PAYLOAD_FILE")

if [[ "$HOOK_TYPE" != "job.completed" ]]; then
  fixture_fail "expected hook payload type job.completed, got $HOOK_TYPE"
fi
if [[ "$HOOK_PLUGIN" != "root_echo" ]]; then
  fixture_fail "expected hook payload plugin root_echo, got $HOOK_PLUGIN"
fi
if [[ "$HOOK_COMMAND" != "poll" ]]; then
  fixture_fail "expected hook payload command poll, got $HOOK_COMMAND"
fi
if [[ "$HOOK_STATUS_FIELD" != "succeeded" ]]; then
  fixture_fail "expected hook payload status succeeded, got $HOOK_STATUS_FIELD"
fi

fixture_log "success"
