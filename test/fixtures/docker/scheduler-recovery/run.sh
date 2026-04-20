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

start_service() {
  fixture_log "starting ductile process"
  "$ROOT_DIR/ductile" system start --config "$CONFIG_DIR" >"$ARTIFACT_DIR/ductile.log" 2>&1 &
  PID=$!
}

wait_ready() {
  local ready=0
  for _ in $(seq 1 40); do
    if curl -fsS http://127.0.0.1:18281/healthz >"$ARTIFACT_DIR/healthz.json" 2>/dev/null; then
      ready=1
      break
    fi
    sleep 0.25
  done
  if [[ "$ready" -ne 1 ]]; then
    fixture_fail "health endpoint did not become ready"
  fi
}

start_service
wait_ready

DB_PATH="$STATE_DIR/ductile.db"
JOB_ID="orphan-job-$(date +%s)"
sqlite3 "$DB_PATH" "INSERT INTO job_queue (id, plugin, command, payload, status, attempt, max_attempts, submitted_by, created_at, started_at) VALUES ('$JOB_ID', 'echo', 'poll', '{}', 'running', 1, 3, 'test-fixture', datetime('now'), datetime('now'));"
printf '%s\n' "$JOB_ID" > "$ARTIFACT_DIR/orphan-job-id.txt"

fixture_log "restarting service to trigger recovery"
kill "$PID"
wait "$PID" || true
PID=""

start_service
wait_ready

for _ in $(seq 1 20); do
  STATUS=$(sqlite3 "$DB_PATH" "SELECT status FROM job_queue WHERE id = '$JOB_ID';" 2>/dev/null || echo "")
  ATTEMPT=$(sqlite3 "$DB_PATH" "SELECT attempt FROM job_queue WHERE id = '$JOB_ID';" 2>/dev/null || echo "")
  if [[ "$STATUS" == "queued" && "$ATTEMPT" == "2" ]]; then
    break
  fi
  sleep 0.25
done
STATUS=$(sqlite3 "$DB_PATH" "SELECT status FROM job_queue WHERE id = '$JOB_ID';")
ATTEMPT=$(sqlite3 "$DB_PATH" "SELECT attempt FROM job_queue WHERE id = '$JOB_ID';")
printf '%s\n' "$STATUS" > "$ARTIFACT_DIR/recovered-status.txt"
printf '%s\n' "$ATTEMPT" > "$ARTIFACT_DIR/recovered-attempt.txt"
if [[ "$STATUS" != "queued" ]]; then
  fixture_fail "expected recovered orphan job status queued, got $STATUS"
fi
if [[ "$ATTEMPT" != "2" ]]; then
  fixture_fail "expected recovered orphan job attempt 2, got $ATTEMPT"
fi

fixture_capture_file "$DB_PATH" state.db
fixture_log "success"
