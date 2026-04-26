#!/usr/bin/env bash
# Sprint 17 fixture: context-aware trigger-level if: predicates.
#
# Verifies that pipeline-level if: predicates can evaluate context.*
# values passed downstream from upstream pipeline baggage claims:
#
#   case A — context available:  upstream claims role=admin,
#            downstream checks context.role eq admin (should fire)
#   case B — context mismatch:   same upstream event, but different
#            downstream checks context.role eq guest (should NOT fire)
#   case C — no context (root):  root trigger with context.role exists
#            predicate should NOT fire (absent context = false)
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
PORT="18518"
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
  local label="$2"
  local body="${3:-{}}"
  local code
  code=$(curl -sS -o "$ARTIFACT_DIR/$label-response.json" -w '%{http_code}' -X POST \
    "http://127.0.0.1:$PORT/pipeline/$pipeline" \
    -H 'Authorization: Bearer test-admin-token' \
    -H 'Content-Type: application/json' \
    --data "$body")
  [[ "$code" == "202" ]] || fixture_fail "$label: pipeline trigger status $code (want 202)"
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

# ---------- case A — context available, predicate TRUE ----------
fixture_log "case A: upstream emits role=admin, downstream with context.role eq admin"
trigger_pipeline upstream_emits_admin_role caseA '{"payload":{"role":"admin"}}'
wait_settled "case A"

A_ADMIN=$(count_plugin downstream_admin)
A_GUEST=$(count_plugin downstream_guest)
printf '%s\n' "$A_ADMIN" >"$ARTIFACT_DIR/caseA-downstream-admin.txt"
printf '%s\n' "$A_GUEST" >"$ARTIFACT_DIR/caseA-downstream-guest.txt"
[[ "$A_ADMIN" == "1" ]] || fixture_fail "case A: downstream_admin = $A_ADMIN, want 1 (context.role eq admin should fire)"
[[ "$A_GUEST" == "0" ]] || fixture_fail "case A: downstream_guest = $A_GUEST, want 0 (context.role neq guest)"

trigger_plugin() {
  # Root jobs (POST /plugin/<name>/handle) emit events with no upstream
  # EventContextID, so SourceContext is nil at the routing boundary.
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

# ---------- case B — absent context routes through Next() with nil context ----------
# emit_no_context is triggered as a ROOT job (no EventContextID), emits
# test.no_context_trigger, dispatcher routes the event via router.Next()
# with SourceContext == nil. The predicate context.role exists must
# evaluate to false → downstream_no_context stays at 0.
fixture_log "case B: root job emits trigger, predicate against absent context.role"
trigger_plugin emit_no_context caseB
wait_settled "case B"

B_NO_CONTEXT=$(count_plugin downstream_no_context)
printf '%s\n' "$B_NO_CONTEXT" >"$ARTIFACT_DIR/caseB-downstream-no-context.txt"
[[ "$B_NO_CONTEXT" == "0" ]] || fixture_fail "case B: downstream_no_context = $B_NO_CONTEXT, want 0 (absent context against context.role exists)"

fixture_log "all cases passed"
