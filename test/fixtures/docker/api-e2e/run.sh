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
PID=""
mkdir -p "$STATE_DIR"
SCENARIO_LOG="$ARTIFACT_DIR/scenario.log"
exec > >(tee "$SCENARIO_LOG") 2>&1

cleanup() {
  fixture_capture_tree "$CONFIG_DIR" config
  fixture_capture_tree "$STATE_DIR" state-dir
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
  if curl -fsS http://127.0.0.1:18181/healthz >"$ARTIFACT_DIR/healthz.json" 2>/dev/null; then
    ready=1
    break
  fi
  sleep 0.25
done
if [[ "$ready" -ne 1 ]]; then
  fixture_fail "health endpoint did not become ready"
fi

fixture_log "triggering direct plugin execution"
PLUGIN_STATUS=$(curl -sS -o "$ARTIFACT_DIR/plugin-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18181/plugin/echo/poll \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"message":"hello"}}')
if [[ "$PLUGIN_STATUS" != "202" ]]; then
  fixture_fail "expected 202 for direct plugin trigger, got $PLUGIN_STATUS"
fi
PLUGIN_JOB_ID=$(jq -r '.job_id' "$ARTIFACT_DIR/plugin-response.json")
if [[ -z "$PLUGIN_JOB_ID" || "$PLUGIN_JOB_ID" == "null" ]]; then
  fixture_fail "plugin trigger returned no job_id"
fi

fixture_log "checking unauthorized request"
UNAUTH_STATUS=$(curl -sS -o "$ARTIFACT_DIR/unauthorized-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18181/plugin/echo/poll \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"message":"hello"}}')
if [[ "$UNAUTH_STATUS" != "401" ]]; then
  fixture_fail "expected 401 for unauthorized request, got $UNAUTH_STATUS"
fi

fixture_log "triggering pipeline execution"
PIPELINE_STATUS=$(curl -sS -o "$ARTIFACT_DIR/pipeline-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18181/pipeline/test-pipeline \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"message":"pipeline"}}')
if [[ "$PIPELINE_STATUS" != "202" ]]; then
  fixture_fail "expected 202 for pipeline trigger, got $PIPELINE_STATUS"
fi
PIPELINE_JOB_ID=$(jq -r '.job_id' "$ARTIFACT_DIR/pipeline-response.json")
if [[ -z "$PIPELINE_JOB_ID" || "$PIPELINE_JOB_ID" == "null" ]]; then
  fixture_fail "pipeline trigger returned no job_id"
fi

DB_PATH="$CONFIG_DIR/state/ductile.db"
for _ in $(seq 1 30); do
  if [[ -f "$DB_PATH" ]]; then
    TOTAL=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_log;" 2>/dev/null || echo "0")
    if [[ "$TOTAL" -ge 2 ]]; then
      break
    fi
  fi
  sleep 0.25
done
TOTAL=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_log;")
echo "$TOTAL" > "$ARTIFACT_DIR/job-log-count.txt"
if [[ "$TOTAL" -lt 2 ]]; then
  fixture_fail "expected at least 2 completed job_log rows, found $TOTAL"
fi

PLUGIN_STATUS_DB=$(sqlite3 "$DB_PATH" "SELECT status FROM job_log WHERE id LIKE '${PLUGIN_JOB_ID}-%' ESCAPE '\\' LIMIT 1;")
PIPELINE_STATUS_DB=$(sqlite3 "$DB_PATH" "SELECT status FROM job_log WHERE id LIKE '${PIPELINE_JOB_ID}-%' ESCAPE '\\' LIMIT 1;")
printf '%s\n' "$PLUGIN_STATUS_DB" > "$ARTIFACT_DIR/plugin-job-status.txt"
printf '%s\n' "$PIPELINE_STATUS_DB" > "$ARTIFACT_DIR/pipeline-job-status.txt"
if [[ -z "$PLUGIN_STATUS_DB" ]]; then
  PLUGIN_STATUS_DB=$(sqlite3 "$DB_PATH" "SELECT status FROM job_log WHERE plugin = 'echo' AND command = 'poll' ORDER BY completed_at DESC LIMIT 1;")
fi
if [[ -z "$PIPELINE_STATUS_DB" ]]; then
  PIPELINE_STATUS_DB=$(sqlite3 "$DB_PATH" "SELECT status FROM job_log WHERE plugin = 'echo' AND command = 'handle' ORDER BY completed_at DESC LIMIT 1;")
fi
if [[ "$PLUGIN_STATUS_DB" != "succeeded" ]]; then
  fixture_fail "expected direct plugin job to succeed, got $PLUGIN_STATUS_DB"
fi
if [[ "$PIPELINE_STATUS_DB" != "failed" ]]; then
  fixture_fail "expected pipeline job to fail with unsupported handle command, got $PIPELINE_STATUS_DB"
fi

fixture_capture_file "$DB_PATH" state.db
fixture_log "success"
