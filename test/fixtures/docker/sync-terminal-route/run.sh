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
  if curl -fsS http://127.0.0.1:18482/healthz >"$ARTIFACT_DIR/healthz.json" 2>/dev/null; then
    ready=1
    break
  fi
  sleep 0.25
done
if [[ "$ready" -ne 1 ]]; then
  fixture_fail "health endpoint did not become ready"
fi

fixture_log "triggering synchronous pipeline with skipped first step"
STATUS_CODE=$(curl -sS -o "$ARTIFACT_DIR/sync-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18482/pipeline/if-sync \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"run_first":false}}')
if [[ "$STATUS_CODE" != "200" ]]; then
  fixture_fail "expected 200 from synchronous pipeline, got $STATUS_CODE"
fi

SYNC_JOB_ID=$(jq -r '.job_id' "$ARTIFACT_DIR/sync-response.json")
SYNC_RESULT=$(jq -r '.result.result' "$ARTIFACT_DIR/sync-response.json")
ROOT_STATUS=$(jq -r '.tree[0].status' "$ARTIFACT_DIR/sync-response.json")

if [[ -z "$SYNC_JOB_ID" || "$SYNC_JOB_ID" == "null" ]]; then
  fixture_fail "sync response returned no job_id"
fi
if [[ "$ROOT_STATUS" != "skipped" ]]; then
  fixture_fail "expected root tree entry to be skipped, got $ROOT_STATUS"
fi
if [[ "$SYNC_RESULT" != "B" ]]; then
  fixture_fail "expected sync terminal result B, got $SYNC_RESULT"
fi

for _ in $(seq 1 40); do
  if [[ -f "$DB_PATH" ]]; then
    TOTAL=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_log;" 2>/dev/null || echo "0")
    if [[ "$TOTAL" -ge 2 ]]; then
      break
    fi
  fi
  sleep 0.25
done

STEP_A_STATUS=$(sqlite3 "$DB_PATH" "SELECT status FROM job_log WHERE plugin = 'step-a' ORDER BY completed_at DESC LIMIT 1;" 2>/dev/null || true)
STEP_B_STATUS=$(sqlite3 "$DB_PATH" "SELECT status FROM job_log WHERE plugin = 'step-b' ORDER BY completed_at DESC LIMIT 1;" 2>/dev/null || true)
printf '%s\n' "$STEP_A_STATUS" >"$ARTIFACT_DIR/step-a-status.txt"
printf '%s\n' "$STEP_B_STATUS" >"$ARTIFACT_DIR/step-b-status.txt"

if [[ "$STEP_B_STATUS" != "succeeded" ]]; then
  fixture_fail "expected terminal step-b job to succeed, got $STEP_B_STATUS"
fi

fixture_log "success"
