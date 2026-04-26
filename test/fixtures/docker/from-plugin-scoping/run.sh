#!/usr/bin/env bash
# Sprint 17 fixture: from_plugin: source selector end-to-end.
#
# Verifies that hook pipelines can scope their match to a specific upstream
# plugin via from_plugin: field:
#
#   case A — scoped to plugin_a:  plugin_a fires hook_scoped_to_a and hook_always
#   case B — not scoped to plugin_a: hook_scoped_to_a does NOT fire for plugin_b
#   case C — scoped to plugin_b:  plugin_b fires hook_scoped_to_b and hook_always
#   case D — not scoped to plugin_b: hook_scoped_to_b does NOT fire for plugin_a
#   case E — no scope (regression): hook_always fires for both plugin_a and plugin_b
#   case F — inspection: /config/view surfaces source_plugin on scoped routes
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
PORT="18517"
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

trigger_plugin() {
  # Hook lifecycle pipelines fire only for ROOT jobs (no EventContextID).
  # Triggering directly via /plugin/<name>/handle keeps plugin_a/plugin_b as
  # root jobs so maybeFireHooks does not short-circuit.
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

# ---------- case A — plugin_a completion fires both scoped and unscoped hooks ----------
fixture_log "case A: plugin_a triggers, hook_scoped_to_a and hook_always should fire"
trigger_plugin plugin_a caseA
wait_settled "case A"

A_SCOPED=$(count_plugin hook_scoped_to_a)
A_ALWAYS=$(count_plugin hook_always)
A_B=$(count_plugin hook_scoped_to_b)
printf '%s\n' "$A_SCOPED" >"$ARTIFACT_DIR/caseA-scoped-to-a.txt"
printf '%s\n' "$A_ALWAYS" >"$ARTIFACT_DIR/caseA-always.txt"
printf '%s\n' "$A_B" >"$ARTIFACT_DIR/caseA-scoped-to-b.txt"
[[ "$A_SCOPED" == "1" ]] || fixture_fail "case A: hook_scoped_to_a = $A_SCOPED, want 1"
[[ "$A_ALWAYS" == "1" ]] || fixture_fail "case A: hook_always = $A_ALWAYS, want 1"
[[ "$A_B" == "0" ]] || fixture_fail "case A: hook_scoped_to_b = $A_B, want 0 (not plugin_b)"

# ---------- case B — plugin_b completion does NOT fire hook_scoped_to_a ----------
fixture_log "case B: plugin_b triggers, hook_scoped_to_a should NOT fire"
trigger_plugin plugin_b caseB
wait_settled "case B"

B_SCOPED_A=$(count_plugin hook_scoped_to_a)
B_SCOPED_B=$(count_plugin hook_scoped_to_b)
B_ALWAYS=$(count_plugin hook_always)
printf '%s\n' "$B_SCOPED_A" >"$ARTIFACT_DIR/caseB-scoped-to-a.txt"
printf '%s\n' "$B_SCOPED_B" >"$ARTIFACT_DIR/caseB-scoped-to-b.txt"
printf '%s\n' "$B_ALWAYS" >"$ARTIFACT_DIR/caseB-always.txt"
[[ "$B_SCOPED_A" == "1" ]] || fixture_fail "case B: hook_scoped_to_a = $B_SCOPED_A, want 1 (no new from case A)"
[[ "$B_SCOPED_B" == "1" ]] || fixture_fail "case B: hook_scoped_to_b = $B_SCOPED_B, want 1"
[[ "$B_ALWAYS" == "2" ]] || fixture_fail "case B: hook_always = $B_ALWAYS, want 2 (incremented from case A)"

# ---------- case F — /config/view inspection surfaces source_plugin ----------
fixture_log "case F: /config/view inspection"
curl -sS "http://127.0.0.1:$PORT/config/view" \
  -H 'Authorization: Bearer test-admin-token' \
  >"$ARTIFACT_DIR/caseF-config-view.json"

# Verify that scoped routes have source_plugin populated
has_scoped_a=$(jq '.compiled_routes.hook_scoped_to_a[0].source.source_plugin' "$ARTIFACT_DIR/caseF-config-view.json")
has_scoped_b=$(jq '.compiled_routes.hook_scoped_to_b[0].source.source_plugin' "$ARTIFACT_DIR/caseF-config-view.json")
has_always=$(jq '.compiled_routes.hook_always[0].source.source_plugin // "null"' "$ARTIFACT_DIR/caseF-config-view.json")

[[ "$has_scoped_a" == '"plugin_a"' ]] || fixture_fail "case F: hook_scoped_to_a source.source_plugin = $has_scoped_a, want \"plugin_a\""
[[ "$has_scoped_b" == '"plugin_b"' ]] || fixture_fail "case F: hook_scoped_to_b source.source_plugin = $has_scoped_b, want \"plugin_b\""
[[ "$has_always" == '"null"' ]] || fixture_fail "case F: hook_always source.source_plugin = $has_always, want null"

fixture_log "all cases passed"
