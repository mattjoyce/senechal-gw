#!/usr/bin/env bash
# Fixture: config-view-redaction
#
# Guards the two secret-redaction guarantees that are actually LIVE at
# runtime (verified via investigation 2026-05-18):
#
#   Part A (F-006, on main) — /config/view redacts sensitive plugin
#     config values at every nesting depth.
#   Part B (C-FRO-15)       — the persisted config snapshot redacts AND
#     fingerprints a secret in a schedule payload (schedules come from
#     cfg.Plugins, which IS populated at runtime).
#   Part C (C-FRO-15)       — rotating only that redacted secret changes
#     the snapshot config_hash (drift signal preserved).
#
# NOTE: C-FRO-15 also covers pipeline step with/baggage + relay subtrees,
# but cfg.Pipelines is never populated at runtime (no loader assigns it;
# pipelines live only in the router). That redaction path is therefore
# dead code in production and is intentionally NOT asserted here — it is
# only reachable from unit tests that hand-build cfg.Pipelines. See the
# C-FRO-16 / C-FRO-15 investigation note.
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
PLUGINS="$CONFIG_DIR/plugins.yaml"
PLUGINS_ORIG="$ARTIFACT_DIR/plugins.yaml.orig"
PORT="18561"
PID=""
rm -rf "$STATE_DIR"
mkdir -p "$STATE_DIR"
cp "$PLUGINS" "$PLUGINS_ORIG"
SCENARIO_LOG="$ARTIFACT_DIR/scenario.log"
exec > >(tee "$SCENARIO_LOG") 2>&1

cleanup() {
  cp "$PLUGINS_ORIG" "$PLUGINS" 2>/dev/null || true
  fixture_capture_tree "$CONFIG_DIR" config
  fixture_capture_file "$DB_PATH" state.db
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

start_service() {
  "$ROOT_DIR/ductile" system start --config "$CONFIG_DIR" >>"$ARTIFACT_DIR/ductile.log" 2>&1 &
  PID=$!
  local ready=0
  for _ in $(seq 1 40); do
    if curl -fsS "http://127.0.0.1:$PORT/healthz" >"$ARTIFACT_DIR/healthz.json" 2>/dev/null; then
      ready=1; break
    fi
    sleep 0.25
  done
  [[ "$ready" == "1" ]] || fixture_fail "health endpoint did not become ready"
}

stop_service() {
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  PID=""
}

latest_snapshot() {
  sqlite3 "$DB_PATH" "SELECT $1 FROM config_snapshots ORDER BY rowid DESC LIMIT 1;"
}

wait_for_snapshot() {
  for _ in $(seq 1 40); do
    local n
    n=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM config_snapshots;" 2>/dev/null || echo 0)
    [[ "${n:-0}" -ge "${1:-1}" ]] && return 0
    sleep 0.25
  done
  fixture_fail "config_snapshots did not reach ${1:-1} row(s)"
}

fixture_log "starting ductile process"
start_service

# ---------- Part A — F-006: /config/view redacts nested plugin config ----------
fixture_log "Part A: /config/view nested plugin-config redaction (F-006)"
curl -sS "http://127.0.0.1:$PORT/config/view" \
  -H 'Authorization: Bearer test-admin-token' \
  >"$ARTIFACT_DIR/config-view.json"

nested=$(jq -r '.plugins.app.config.nested.api_key' "$ARTIFACT_DIR/config-view.json")
public=$(jq -r '.plugins.app.config.public' "$ARTIFACT_DIR/config-view.json")
[[ "$nested" == "[REDACTED]" ]] || fixture_fail "Part A: nested.api_key = '$nested', want '[REDACTED]'"
[[ "$public" == "visible-value" ]] || fixture_fail "Part A: public = '$public', want 'visible-value' (over-redaction)"
if grep -q "PLAINTEXT_PLUGIN_NESTED" "$ARTIFACT_DIR/config-view.json"; then
  fixture_fail "Part A: /config/view leaked the nested plugin secret in plaintext"
fi
fixture_log "Part A OK — nested secret redacted, non-secret preserved"

# ---------- Part B — C-FRO-15: snapshot redacts + fingerprints schedule payload ----------
fixture_log "Part B: config snapshot schedule-payload redaction + fingerprint (C-FRO-15)"
wait_for_snapshot 1
latest_snapshot sanitized_config    >"$ARTIFACT_DIR/sanitized_config.json"
latest_snapshot secret_fingerprints >"$ARTIFACT_DIR/secret_fingerprints.json"

if grep -q "PLAINTEXT_SCHED_PAYLOAD" "$ARTIFACT_DIR/sanitized_config.json"; then
  fixture_fail "Part B: snapshot leaked the schedule payload secret in plaintext"
fi
purposes=$(jq -r '.[].purpose' "$ARTIFACT_DIR/secret_fingerprints.json")
printf '%s\n' "$purposes" >"$ARTIFACT_DIR/secret-purposes.txt"
echo "$purposes" | grep -qE 'schedules\[0\]\.payload\.token' \
  || fixture_fail "Part B: schedule payload.token not in secret_fingerprints"
fixture_log "Part B OK — schedule-payload secret redacted AND fingerprinted"

# ---------- Part C — C-FRO-15: secret-only rotation flips config_hash ----------
fixture_log "Part C: secret-only rotation changes snapshot config_hash"
first_hash=$(latest_snapshot config_hash)
printf '%s\n' "$first_hash" >"$ARTIFACT_DIR/first-config-hash.txt"

stop_service
sed 's/PLAINTEXT_SCHED_PAYLOAD/PLAINTEXT_SCHED_PAYLOAD_ROTATED/' "$PLUGINS_ORIG" >"$PLUGINS"
start_service
wait_for_snapshot 2

second_hash=$(latest_snapshot config_hash)
printf '%s\n' "$second_hash" >"$ARTIFACT_DIR/second-config-hash.txt"
latest_snapshot sanitized_config >"$ARTIFACT_DIR/sanitized_config_after.json"
if grep -q "PLAINTEXT_SCHED_PAYLOAD_ROTATED" "$ARTIFACT_DIR/sanitized_config_after.json"; then
  fixture_fail "Part C: rotated secret leaked in plaintext after restart"
fi
[[ -n "$first_hash" && -n "$second_hash" ]] || fixture_fail "Part C: missing config_hash values"
[[ "$first_hash" != "$second_hash" ]] \
  || fixture_fail "Part C: config_hash unchanged after secret-only rotation ($first_hash) — drift not tracked"

fixture_log "Part C OK — config_hash changed on secret-only rotation"
fixture_log "all parts passed"
