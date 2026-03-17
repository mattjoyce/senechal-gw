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
WATCH_DIR="/tmp/ductile-watch-test"
PID=""
rm -rf "$STATE_DIR"
rm -rf "$WATCH_DIR"
mkdir -p "$STATE_DIR"
mkdir -p "$WATCH_DIR"
SCENARIO_LOG="$ARTIFACT_DIR/scenario.log"
exec > >(tee "$SCENARIO_LOG") 2>&1

cleanup() {
  fixture_capture_tree "$CONFIG_DIR" config
  fixture_capture_tree "$STATE_DIR" state-dir
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -rf "$WATCH_DIR"/*.txt
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

fixture_log "creating file in watched directory"
echo "hello watch" > "$WATCH_DIR/trigger.txt"

fixture_log "waiting for folder_watch to detect file (should take ~1s tick + 1s poll)"
DB_PATH="$STATE_DIR/ductile.db"
detected=0
for _ in $(seq 1 80); do
  if [[ -f "$DB_PATH" ]]; then
    # Check for the job that was triggered by the pipeline (echo plugin)
    COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_log WHERE plugin = 'echo' AND status = 'succeeded';" 2>/dev/null || echo "0")
    if [[ "$COUNT" -gt 0 ]]; then
      detected=1
      break
    fi
  fi
  sleep 0.25
done

if [[ "$detected" -ne 1 ]]; then
  # Check if poll happened at all
  POLL_COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_log WHERE plugin = 'folder_watch';" 2>/dev/null || echo "0")
  fixture_log "folder_watch poll count: $POLL_COUNT"
  fixture_fail "folder_watch did not detect the new file"
fi

fixture_log "file change detected successfully"
fixture_log "success"
