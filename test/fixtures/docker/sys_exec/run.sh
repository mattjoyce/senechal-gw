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
rm -rf "$STATE_DIR"
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

fixture_log "triggering test-pipeline via API"
TRIGGER_STATUS=$(curl -sS -o "$ARTIFACT_DIR/trigger-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18181/pipeline/test-pipeline \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"message":"hello from sys_exec"}}')

if [[ "$TRIGGER_STATUS" != "202" ]]; then
  fixture_fail "expected 202 for plugin trigger, got $TRIGGER_STATUS"
fi
JOB_ID=$(jq -r '.job_id' "$ARTIFACT_DIR/trigger-response.json")
fixture_log "job_id: $JOB_ID"

# Wait for job completion
DB_PATH="$STATE_DIR/ductile.db"
job_done=0
for _ in $(seq 1 40); do
  if [[ -f "$DB_PATH" ]]; then
    STATUS_DB=$(sqlite3 "$DB_PATH" "SELECT status FROM job_log WHERE id LIKE '${JOB_ID}-%' LIMIT 1;" 2>/dev/null || true)
    if [[ "$STATUS_DB" == "succeeded" || "$STATUS_DB" == "failed" ]]; then
      job_done=1
      break
    fi
  fi
  sleep 0.25
done

if [[ "$job_done" -ne 1 ]]; then
  fixture_fail "job did not complete within timeout"
fi
fixture_log "job completed with status: $STATUS_DB"

if [[ "$STATUS_DB" != "succeeded" ]]; then
  fixture_fail "expected success, got $STATUS_DB"
fi

# Verify output.txt landed at the plugin's configured working_dir.
# As of Sprint 18 the core no longer provisions a workspace; the plugin
# config (plugins.yaml: sys_exec.config.working_dir) is the source of
# truth for where the spawned subprocess runs.
OUT_FILE="$STATE_DIR/output.txt"

fixture_log "checking output file at $OUT_FILE"
if [[ ! -f "$OUT_FILE" ]]; then
  fixture_fail "output.txt not found at $OUT_FILE"
fi

CONTENT=$(cat "$OUT_FILE")
if [[ "$CONTENT" != "hello from sys_exec" ]]; then
  fixture_fail "unexpected content in output.txt: '$CONTENT'"
fi

fixture_log "success"

# Suppress unused-variable warning when JOB_ID is captured but not asserted.
: "$JOB_ID"
