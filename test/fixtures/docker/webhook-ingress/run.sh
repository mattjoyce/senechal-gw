#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:?}"
FIXTURE_NAME="${FIXTURE_NAME:?}"
ARTIFACT_ROOT="${ARTIFACT_ROOT:?}"
# shellcheck source=/dev/null
source "$ROOT_DIR/scripts/test-docker-lib"
fixture_init

CONFIG_SRC="$FIXTURE_DIR/config"
CONFIG_DIR="$ARTIFACT_DIR/runtime-config"
STATE_DIR="$CONFIG_DIR/state"
PID=""
rm -rf "$CONFIG_DIR"
mkdir -p "$CONFIG_DIR"
cp -R "$CONFIG_SRC"/. "$CONFIG_DIR/"
mkdir -p "$STATE_DIR"
SCENARIO_LOG="$ARTIFACT_DIR/scenario.log"
exec > >(tee "$SCENARIO_LOG") 2>&1

cleanup() {
  fixture_capture_tree "$STATE_DIR" state-dir
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

fixture_log "refreshing fixture checksums"
"$ROOT_DIR/ductile" config lock --config-dir "$CONFIG_DIR" >"$ARTIFACT_DIR/config-lock.log" 2>&1

fixture_log "starting ductile process"
"$ROOT_DIR/ductile" system start --config "$CONFIG_DIR" >"$ARTIFACT_DIR/ductile.log" 2>&1 &
PID=$!

ready=0
for _ in $(seq 1 40); do
  if curl -fsS http://127.0.0.1:18081/healthz >"$ARTIFACT_DIR/healthz.json" 2>/dev/null; then
    ready=1
    break
  fi
  sleep 0.25
done
if [[ "$ready" -ne 1 ]]; then
  fixture_fail "health endpoint did not become ready"
fi

fixture_log "sending valid webhook"
BODY='{"event":"push"}'
SIG=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac 'test-secret' -hex | awk '{print $2}')
VALID_STATUS=$(curl -sS -o "$ARTIFACT_DIR/valid-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18091/webhook/github \
  -H "X-Hub-Signature-256: sha256=$SIG" \
  -H 'Content-Type: application/json' \
  --data "$BODY")
if [[ "$VALID_STATUS" != "202" ]]; then
  fixture_fail "expected 202 for valid webhook, got $VALID_STATUS"
fi

fixture_log "sending invalid webhook"
INVALID_STATUS=$(curl -sS -o "$ARTIFACT_DIR/invalid-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18091/webhook/github \
  -H 'X-Hub-Signature-256: sha256=deadbeef' \
  -H 'Content-Type: application/json' \
  --data "$BODY")
if [[ "$INVALID_STATUS" != "403" ]]; then
  fixture_fail "expected 403 for invalid webhook, got $INVALID_STATUS"
fi

DB_PATH="$STATE_DIR/ductile.db"
for _ in $(seq 1 20); do
  if [[ -f "$DB_PATH" ]]; then
    COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_queue WHERE plugin = 'echo' AND command = 'handle';" 2>/dev/null || echo "0")
    if [[ "$COUNT" -ge 1 ]]; then
      break
    fi
  fi
  sleep 0.25
done
COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_queue WHERE plugin = 'echo' AND command = 'handle';")
echo "$COUNT" > "$ARTIFACT_DIR/job-count.txt"
if [[ "$COUNT" -lt 1 ]]; then
  fixture_fail "expected queued webhook job, found $COUNT"
fi

fixture_capture_file "$DB_PATH" state.db
fixture_log "success"
