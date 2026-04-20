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
WS_DIR="$STATE_DIR/workspaces"
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

# ── Scenario 1: trigger a job and get its job_id ─────────────────────────────
fixture_log "triggering echo/poll via API"
TRIGGER_STATUS=$(curl -sS -o "$ARTIFACT_DIR/trigger-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18181/plugin/echo/poll \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"message":"workspace-lifecycle-test"}}')
if [[ "$TRIGGER_STATUS" != "202" ]]; then
  fixture_fail "expected 202 for plugin trigger, got $TRIGGER_STATUS"
fi
JOB_ID=$(jq -r '.job_id' "$ARTIFACT_DIR/trigger-response.json")
if [[ -z "$JOB_ID" || "$JOB_ID" == "null" ]]; then
  fixture_fail "trigger returned no job_id"
fi
fixture_log "job_id: $JOB_ID"

# ── Wait for job to complete ──────────────────────────────────────────────────
DB_PATH="$STATE_DIR/ductile.db"
job_done=0
for _ in $(seq 1 40); do
  if [[ -f "$DB_PATH" ]]; then
    STATUS_DB=$(sqlite3 "$DB_PATH" \
      "SELECT status FROM job_log WHERE id LIKE '${JOB_ID}-%' LIMIT 1;" 2>/dev/null || true)
    if [[ "$STATUS_DB" == "succeeded" || "$STATUS_DB" == "failed" ]]; then
      job_done=1
      break
    fi
  fi
  sleep 0.25
done
if [[ "$job_done" -ne 1 ]]; then
  fixture_fail "job did not complete within timeout"
fi
fixture_log "job completed with status: $STATUS_DB"

# ── Scenario 2: assert sharded workspace layout ───────────────────────────────
SHARD="${JOB_ID:0:2}"
WS_PATH="$WS_DIR/$SHARD/$JOB_ID"
fixture_log "checking sharded workspace: workspaces/$SHARD/$JOB_ID"
if [[ ! -d "$WS_PATH" ]]; then
  fixture_fail "expected sharded workspace directory at $WS_PATH"
fi
printf '%s\n' "$WS_PATH" >"$ARTIFACT_DIR/workspace-path.txt"
fixture_log "sharded workspace exists"

# ── Scenario 3: assert black box bundle ──────────────────────────────────────
BUNDLE_DIR="$WS_PATH/.ductile"
META_FILE="$BUNDLE_DIR/metadata.json"
fixture_log "checking black box bundle at $BUNDLE_DIR"
if [[ ! -d "$BUNDLE_DIR" ]]; then
  fixture_fail "expected .ductile/ bundle directory inside workspace"
fi
if [[ ! -f "$META_FILE" ]]; then
  fixture_fail "expected metadata.json in .ductile/ bundle"
fi

# Validate JSON and check required fields
cp "$META_FILE" "$ARTIFACT_DIR/metadata.json"
META_JOB_ID=$(jq -r '.job_id // empty' "$META_FILE")
META_STATUS=$(jq -r '.status // empty' "$META_FILE")
META_PLUGIN=$(jq -r '.plugin // empty' "$META_FILE")
META_COMMAND=$(jq -r '.command // empty' "$META_FILE")
if [[ -z "$META_JOB_ID" ]]; then
  fixture_fail "metadata.json missing job_id"
fi
if [[ -z "$META_STATUS" ]]; then
  fixture_fail "metadata.json missing status"
fi
if [[ -z "$META_PLUGIN" ]]; then
  fixture_fail "metadata.json missing plugin"
fi
if [[ -z "$META_COMMAND" ]]; then
  fixture_fail "metadata.json missing command"
fi
if [[ "$META_PLUGIN" != "echo" ]]; then
  fixture_fail "metadata.json plugin expected 'echo', got '$META_PLUGIN'"
fi
if [[ "$META_COMMAND" != "poll" ]]; then
  fixture_fail "metadata.json command expected 'poll', got '$META_COMMAND'"
fi
if [[ "$META_STATUS" != "succeeded" ]]; then
  fixture_fail "metadata.json status expected 'succeeded', got '$META_STATUS'"
fi
fixture_log "black box bundle valid (plugin=$META_PLUGIN command=$META_COMMAND status=$META_STATUS)"

# ── Scenario 4: janitor prunes stale workspaces ───────────────────────────────
# Create a fake workspace dir with an old mtime (2 minutes ago) so the janitor
# (TTL=30s, tick=1s) prunes it on the next tick.
STALE_ID="st-a1e-workspace-janitor-test"
STALE_SHARD="${STALE_ID:0:2}"
STALE_PATH="$WS_DIR/$STALE_SHARD/$STALE_ID"
mkdir -p "$STALE_PATH"
touch -d "2 minutes ago" "$STALE_PATH"
fixture_log "created stale workspace: workspaces/$STALE_SHARD/$STALE_ID"

# Wait up to 5s for the janitor to clean it up
pruned=0
for _ in $(seq 1 20); do
  if [[ ! -d "$STALE_PATH" ]]; then
    pruned=1
    break
  fi
  sleep 0.25
done
if [[ "$pruned" -ne 1 ]]; then
  fixture_fail "janitor did not prune stale workspace within timeout"
fi
fixture_log "janitor pruned stale workspace"

# Real job workspace must still be intact
if [[ ! -d "$WS_PATH" ]]; then
  fixture_fail "janitor incorrectly pruned the live job workspace"
fi
fixture_log "live job workspace still intact"

fixture_capture_file "$DB_PATH" state.db
fixture_log "success"
