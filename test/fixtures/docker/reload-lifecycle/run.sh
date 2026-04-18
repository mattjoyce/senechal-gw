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
SCENARIO_LOG="$ARTIFACT_DIR/scenario.log"

rm -rf "$CONFIG_DIR"
mkdir -p "$CONFIG_DIR"
cp -R "$CONFIG_SRC"/. "$CONFIG_DIR/"
mkdir -p "$CONFIG_DIR/plugins/echo" "$STATE_DIR"
cp "$ROOT_DIR/plugins/echo/manifest.yaml" "$CONFIG_DIR/plugins/echo/manifest.yaml"
cp "$ROOT_DIR/plugins/echo/run.sh" "$CONFIG_DIR/plugins/echo/run.sh"

exec > >(tee "$SCENARIO_LOG") 2>&1

cleanup() {
  fixture_capture_tree "$STATE_DIR" state-dir
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

wait_ready() {
  local label="$1"
  local ready=0
  for _ in $(seq 1 40); do
    if curl -fsS --max-time 1 http://127.0.0.1:18381/healthz >"$ARTIFACT_DIR/healthz-${label}.json" 2>/dev/null; then
      ready=1
      break
    fi
    sleep 0.25
  done
  if [[ "$ready" -ne 1 ]]; then
    fixture_fail "health endpoint did not become ready after $label"
  fi
}

reload_gateway() {
  local label="$1"
  fixture_log "requesting reload: $label"
  local status
  if ! status=$(curl -sS --max-time 8 -o "$ARTIFACT_DIR/reload-${label}.json" -w '%{http_code}' -X POST \
    http://127.0.0.1:18381/system/reload \
    -H 'Authorization: Bearer test-admin-token'); then
    fixture_fail "reload $label request failed"
  fi
  if [[ "$status" != "200" ]]; then
    fixture_fail "expected reload $label to return 200, got $status"
  fi
  local reload_status
  reload_status=$(jq -r '.status // empty' "$ARTIFACT_DIR/reload-${label}.json")
  if [[ "$reload_status" != "ok" ]]; then
    fixture_fail "expected reload $label status ok, got $reload_status"
  fi
}

trigger_plugin() {
  local label="$1"
  fixture_log "triggering echo after $label"
  local status
  if ! status=$(curl -sS --max-time 5 -o "$ARTIFACT_DIR/plugin-${label}.json" -w '%{http_code}' -X POST \
    http://127.0.0.1:18381/plugin/echo/poll \
    -H 'Authorization: Bearer test-admin-token' \
    -H 'Content-Type: application/json' \
    --data '{"payload":{"message":"reload-lifecycle"}}'); then
    fixture_fail "plugin trigger after $label request failed"
  fi
  if [[ "$status" != "202" ]]; then
    fixture_fail "expected plugin trigger after $label to return 202, got $status"
  fi
  local job_id
  job_id=$(jq -r '.job_id // empty' "$ARTIFACT_DIR/plugin-${label}.json")
  if [[ -z "$job_id" ]]; then
    fixture_fail "plugin trigger after $label returned no job_id"
  fi
}

fixture_log "starting ductile process"
"$ROOT_DIR/ductile" system start --config "$CONFIG_DIR" >"$ARTIFACT_DIR/ductile.log" 2>&1 &
PID=$!

wait_ready "initial"
trigger_plugin "initial-start"

perl -0pi -e 's/log_level: info/log_level: debug/' "$CONFIG_DIR/config.yaml"

reload_gateway "first"
wait_ready "first-reload"
trigger_plugin "first-reload"

reload_gateway "second"
wait_ready "second-reload"
trigger_plugin "second-reload"

if ! kill -0 "$PID" 2>/dev/null; then
  fixture_fail "ductile process exited during reload lifecycle"
fi

DB_PATH="$STATE_DIR/ductile.db"
for _ in $(seq 1 30); do
  if [[ -f "$DB_PATH" ]]; then
    TOTAL=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_log WHERE plugin = 'echo' AND command = 'poll';" 2>/dev/null || echo "0")
    if [[ "$TOTAL" -ge 3 ]]; then
      break
    fi
  fi
  sleep 0.25
done
TOTAL=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_log WHERE plugin = 'echo' AND command = 'poll';")
printf '%s\n' "$TOTAL" > "$ARTIFACT_DIR/echo-job-log-count.txt"
if [[ "$TOTAL" -lt 3 ]]; then
  fixture_fail "expected at least 3 echo job logs across reloads, found $TOTAL"
fi

fixture_capture_file "$DB_PATH" state.db
fixture_log "success"
