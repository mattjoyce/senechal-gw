#!/usr/bin/env bash
# Hickey Sprint 16 fixture: pipeline-level if: predicate end-to-end.
#
# Verifies the new trigger-level if: predicate against a real ductile
# binary using the standard test-docker harness:
#
#   case A — sanity:        unguarded trigger fires its consumer
#   case B — predicate true:  guarded trigger + kind=workout fires
#   case C — predicate false: guarded trigger + kind=weight does NOT
#   case D — hook scope:      on-hook + if: gates job.completed at
#                             trigger time so only guarded_step
#                             completion routes to the hook pipeline
#
# NOTE: each consumer has its own trigger event name to sidestep the
# multi-consumer fan-out dedupe documented in beads ductile-7m4.
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
PORT="18516"
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

trigger_pipeline() {
  local pipeline="$1"
  local body="$2"
  local label="$3"
  local code
  code=$(curl -sS -o "$ARTIFACT_DIR/$label-response.json" -w '%{http_code}' -X POST \
    "http://127.0.0.1:$PORT/pipeline/$pipeline" \
    -H 'Authorization: Bearer test-admin-token' \
    -H 'Content-Type: application/json' \
    --data "$body")
  [[ "$code" == "202" ]] || fixture_fail "$label: pipeline trigger status $code (want 202)"
}

trigger_plugin() {
  local plugin="$1"
  local label="$2"
  local code
  code=$(curl -sS -o "$ARTIFACT_DIR/$label-response.json" -w '%{http_code}' -X POST \
    "http://127.0.0.1:$PORT/plugin/$plugin/handle" \
    -H 'Authorization: Bearer test-admin-token' \
    -H 'Content-Type: application/json' \
    --data '{}')
  [[ "$code" == "202" ]] || fixture_fail "$label: plugin trigger status $code (want 202)"
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

# ---------- case A — unguarded sanity ----------
fixture_log "case A: unguarded trigger (no predicate)"
trigger_pipeline starter-unguarded '{"payload":{"kind":"weight"}}' caseA
wait_settled "case A"

A_EMIT_U=$(count_plugin emit_event_unguarded)
A_ALWAYS=$(count_plugin always_step)
printf '%s\n' "$A_EMIT_U" >"$ARTIFACT_DIR/caseA-emit-unguarded.txt"
printf '%s\n' "$A_ALWAYS" >"$ARTIFACT_DIR/caseA-always.txt"
[[ "$A_EMIT_U" == "1" ]] || fixture_fail "case A: emit_event_unguarded count = $A_EMIT_U, want 1"
[[ "$A_ALWAYS" == "1" ]] || fixture_fail "case A: always_step count = $A_ALWAYS, want 1"

# ---------- case B — predicate TRUE ----------
fixture_log "case B: guarded trigger, kind=workout (predicate TRUE)"
trigger_pipeline starter-guarded '{"payload":{"kind":"workout"}}' caseB
wait_settled "case B"

B_EMIT_G=$(count_plugin emit_event_guarded)
B_GUARDED=$(count_plugin guarded_step)
printf '%s\n' "$B_EMIT_G"  >"$ARTIFACT_DIR/caseB-emit-guarded.txt"
printf '%s\n' "$B_GUARDED" >"$ARTIFACT_DIR/caseB-guarded.txt"
[[ "$B_EMIT_G"  == "1" ]] || fixture_fail "case B: emit_event_guarded = $B_EMIT_G, want 1"
[[ "$B_GUARDED" == "1" ]] || fixture_fail "case B: guarded_step = $B_GUARDED, want 1 (predicate-true should dispatch)"

# ---------- case C — predicate FALSE ----------
fixture_log "case C: guarded trigger, kind=weight (predicate FALSE)"
trigger_pipeline starter-guarded '{"payload":{"kind":"weight"}}' caseC
wait_settled "case C"

C_EMIT_G=$(count_plugin emit_event_guarded)
C_GUARDED=$(count_plugin guarded_step)
printf '%s\n' "$C_EMIT_G"  >"$ARTIFACT_DIR/caseC-emit-guarded.txt"
printf '%s\n' "$C_GUARDED" >"$ARTIFACT_DIR/caseC-guarded.txt"
[[ "$C_EMIT_G"  == "2" ]] || fixture_fail "case C: emit_event_guarded = $C_EMIT_G, want 2"
[[ "$C_GUARDED" == "1" ]] || fixture_fail "case C: guarded_step = $C_GUARDED, want 1 (predicate-false must NOT dispatch)"

# ---------- case D — hook scope ----------
# Hooks fire only for ROOT jobs (no event_context_id) whose plugin
# config has notify_on_complete: true. Trigger directly via /plugin
# so the completions are root-level and the hook pipeline fires.
fixture_log "case D: direct root-job triggers for hook predicate matrix"
trigger_plugin guarded_step caseD-guarded
trigger_plugin always_step  caseD-always
wait_settled "case D"

HOOK_TOTAL=$(count_plugin hook_step)
printf '%s\n' "$HOOK_TOTAL" >"$ARTIFACT_DIR/caseD-hook-total.txt"
if [[ "$HOOK_TOTAL" != "1" ]]; then
  sqlite3 "$DB_PATH" \
    "SELECT plugin, COUNT(*) FROM job_queue WHERE status='succeeded' GROUP BY plugin;" \
    >"$ARTIFACT_DIR/caseD-completion-breakdown.txt" 2>&1 || true
  fixture_fail "case D: hook_step = $HOOK_TOTAL, want 1 (predicate scope leak)"
fi

fixture_log "success — pipeline-level if: predicate verified"
fixture_log "  case A: unguarded sanity (always fires)        ✓"
fixture_log "  case B: predicate TRUE  → guarded_step ran     ✓"
fixture_log "  case C: predicate FALSE → guarded_step skipped ✓"
fixture_log "  case D: hook predicate gates job.completed     ✓"
