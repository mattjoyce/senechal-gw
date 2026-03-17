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
mkdir -p "/tmp/ductile-test-fh"
SCENARIO_LOG="$ARTIFACT_DIR/scenario.log"
exec > >(tee "$SCENARIO_LOG") 2>&1

cleanup() {
  fixture_capture_tree "$CONFIG_DIR" config
  fixture_capture_tree "$STATE_DIR" state-dir
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -rf "/tmp/ductile-test-fh"
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

fixture_log "scenario: write file (allowed path)"
WRITE_STATUS=$(curl -sS -o "$ARTIFACT_DIR/write-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18181/plugin/file_handler/handle \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"action":"write", "content":"hello file_handler", "output_path":"/tmp/ductile-test-fh/test.txt"}}')

if [[ "$WRITE_STATUS" != "202" ]]; then
  fixture_fail "expected 202 for write action, got $WRITE_STATUS"
fi
JOB_ID=$(jq -r '.job_id' "$ARTIFACT_DIR/write-response.json")

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

if [[ "$STATUS_DB" != "succeeded" ]]; then
  fixture_fail "write job failed: $STATUS_DB"
fi

if [[ ! -f "/tmp/ductile-test-fh/test.txt" ]]; then
  fixture_fail "file was not written to disk"
fi
if [[ "$(cat /tmp/ductile-test-fh/test.txt)" != "hello file_handler" ]]; then
  fixture_fail "file content mismatch"
fi
fixture_log "write OK"

fixture_log "scenario: read file (allowed path)"
READ_STATUS=$(curl -sS -o "$ARTIFACT_DIR/read-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18181/plugin/file_handler/handle \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"action":"read", "file_path":"/tmp/ductile-test-fh/test.txt"}}')

if [[ "$READ_STATUS" != "202" ]]; then
  fixture_fail "expected 202 for read action, got $READ_STATUS"
fi
JOB_ID=$(jq -r '.job_id' "$ARTIFACT_DIR/read-response.json")

# Wait for job completion
job_done=0
for _ in $(seq 1 40); do
  STATUS_DB=$(sqlite3 "$DB_PATH" "SELECT status FROM job_log WHERE id LIKE '${JOB_ID}-%' LIMIT 1;" 2>/dev/null || true)
  if [[ "$STATUS_DB" == "succeeded" || "$STATUS_DB" == "failed" ]]; then
    job_done=1
    break
  fi
  sleep 0.25
done

if [[ "$STATUS_DB" != "succeeded" ]]; then
  fixture_fail "read job failed: $STATUS_DB"
fi

RESULT_JSON=$(sqlite3 "$DB_PATH" "SELECT result FROM job_log WHERE id LIKE '${JOB_ID}-%' LIMIT 1;")
# file_handler returns content in event payload, not in top-level result
CONTENT_OUT=$(echo "$RESULT_JSON" | jq -r '.events[0].payload.content')
if [[ "$CONTENT_OUT" != "hello file_handler" ]]; then
  fixture_fail "read content mismatch: got '$CONTENT_OUT'"
fi
fixture_log "read OK"

fixture_log "scenario: write file (blocked path)"
BLOCK_STATUS=$(curl -sS -o "$ARTIFACT_DIR/blocked-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18181/plugin/file_handler/handle \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"action":"write", "content":"hacker", "output_path":"/etc/passwd"}}')

if [[ "$BLOCK_STATUS" != "202" ]]; then
  fixture_fail "expected 202 for trigger, got $BLOCK_STATUS"
fi
JOB_ID=$(jq -r '.job_id' "$ARTIFACT_DIR/blocked-response.json")

# Wait for job completion
job_done=0
for _ in $(seq 1 40); do
  STATUS_DB=$(sqlite3 "$DB_PATH" "SELECT status FROM job_log WHERE id LIKE '${JOB_ID}-%' LIMIT 1;" 2>/dev/null || true)
  if [[ "$STATUS_DB" == "succeeded" || "$STATUS_DB" == "failed" ]]; then
    job_done=1
    break
  fi
  sleep 0.25
done

if [[ "$STATUS_DB" != "failed" ]]; then
  fixture_fail "expected job to fail for blocked path, got $STATUS_DB"
fi
fixture_log "security check OK"

fixture_log "success"
