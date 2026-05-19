#!/usr/bin/env bash
# Fixture: fanout-dedupe-scope
#
# Regression guard for C-FRO-16 (queue dedupe namespace scoped by target
# plugin+command). A single source event (work.ready) fans out to two
# distinct consumer pipelines. Before the fix the two sibling jobs
# inherited the source event's dedupe key WITHOUT target scoping, so the
# second collapsed as a duplicate and its work was silently lost — the
# ductile-7m4 multi-consumer fan-out collapse that other fixtures avoid
# by using one starter per consumer. This fixture does the opposite on
# purpose and asserts BOTH branches run.
#
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
PORT="18562"
PID=""
rm -rf "$STATE_DIR"
mkdir -p "$STATE_DIR"
SCENARIO_LOG="$ARTIFACT_DIR/scenario.log"
exec > >(tee "$SCENARIO_LOG") 2>&1

cleanup() {
  fixture_capture_tree "$CONFIG_DIR" config
  fixture_capture_file "$DB_PATH" state.db
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

wait_settled() {
  local label="$1"
  local stable=0
  for _ in $(seq 1 240); do
    if [[ -f "$DB_PATH" ]]; then
      local unsettled
      unsettled=$(sqlite3 "$DB_PATH" \
        "SELECT COUNT(*) FROM job_queue WHERE status IN ('queued','running');" 2>/dev/null || echo 1)
      if [[ "$unsettled" == "0" ]]; then
        stable=$((stable+1))
        [[ "$stable" -ge 4 ]] && return 0
      else
        stable=0
      fi
    fi
    sleep 0.25
  done
  fixture_fail "$label: queue did not settle"
}

count_plugin() {
  sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_queue WHERE plugin='$1';"
}
count_plugin_status() {
  sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM job_queue WHERE plugin='$1' AND status='$2';"
}

fixture_log "starting ductile process"
"$ROOT_DIR/ductile" system start --config "$CONFIG_DIR" >"$ARTIFACT_DIR/ductile.log" 2>&1 &
PID=$!

ready=0
for _ in $(seq 1 40); do
  if curl -fsS "http://127.0.0.1:$PORT/healthz" >"$ARTIFACT_DIR/healthz.json" 2>/dev/null; then
    ready=1; break
  fi
  sleep 0.25
done
[[ "$ready" == "1" ]] || fixture_fail "health endpoint did not become ready"

fixture_log "triggering single starter -> one work.ready event -> two consumers"
code=$(curl -sS -o "$ARTIFACT_DIR/trigger-response.json" -w '%{http_code}' -X POST \
  "http://127.0.0.1:$PORT/pipeline/start-fanout" \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{}}')
[[ "$code" == "202" ]] || fixture_fail "starter trigger status $code (want 202)"

wait_settled "fan-out"

EMIT=$(count_plugin emitter)
A=$(count_plugin branch_a)
B=$(count_plugin branch_b)
A_OK=$(count_plugin_status branch_a succeeded)
B_OK=$(count_plugin_status branch_b succeeded)
printf 'emitter=%s branch_a=%s branch_b=%s a_ok=%s b_ok=%s\n' \
  "$EMIT" "$A" "$B" "$A_OK" "$B_OK" >"$ARTIFACT_DIR/counts.txt"
sqlite3 "$DB_PATH" \
  "SELECT plugin,command,status,dedupe_key FROM job_queue ORDER BY rowid;" \
  >"$ARTIFACT_DIR/job-queue.txt" 2>/dev/null || true

[[ "$EMIT" == "1" ]] || fixture_fail "emitter ran $EMIT times, want 1"
# The core C-FRO-16 assertion: distinct fan-out targets must NOT collapse.
[[ "$A" == "1" ]] || fixture_fail "branch_a jobs = $A, want 1 (fan-out target collapsed?)"
[[ "$B" == "1" ]] || fixture_fail "branch_b jobs = $B, want 1 (fan-out target collapsed — C-FRO-16 regression)"
[[ "$A_OK" == "1" ]] || fixture_fail "branch_a not succeeded ($A_OK/1)"
[[ "$B_OK" == "1" ]] || fixture_fail "branch_b not succeeded ($B_OK/1)"

fixture_log "success — both fan-out branches ran (no dedupe collapse)"
