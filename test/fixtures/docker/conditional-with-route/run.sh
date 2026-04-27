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
DB_PATH="$STATE_DIR/ductile.db"
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

wait_for_tree() {
  local root_job_id="$1"
  local expected_count="$2"
  local label="$3"

  for _ in $(seq 1 80); do
    if [[ -f "$DB_PATH" ]]; then
      local count
      local unsettled
      count=$(sqlite3 "$DB_PATH" "
WITH RECURSIVE tree(id, status) AS (
  SELECT id, status
  FROM job_queue
  WHERE id = '$root_job_id'
  UNION ALL
  SELECT jq.id, jq.status
  FROM job_queue jq
  JOIN tree ON jq.parent_job_id = tree.id
)
SELECT COUNT(*) FROM tree;
")
      unsettled=$(sqlite3 "$DB_PATH" "
WITH RECURSIVE tree(id, status) AS (
  SELECT id, status
  FROM job_queue
  WHERE id = '$root_job_id'
  UNION ALL
  SELECT jq.id, jq.status
  FROM job_queue jq
  JOIN tree ON jq.parent_job_id = tree.id
)
SELECT COUNT(*) FROM tree WHERE status IN ('queued', 'running');
")
      if [[ "$count" == "$expected_count" && "$unsettled" == "0" ]]; then
        return 0
      fi
    fi
    sleep 0.25
  done

  fixture_fail "$label tree did not settle to $expected_count jobs"
}

tree_plugin_count() {
  local root_job_id="$1"
  local plugin_name="$2"
  sqlite3 "$DB_PATH" "
WITH RECURSIVE tree(id, plugin) AS (
  SELECT id, plugin
  FROM job_queue
  WHERE id = '$root_job_id'
  UNION ALL
  SELECT jq.id, jq.plugin
  FROM job_queue jq
  JOIN tree ON jq.parent_job_id = tree.id
)
SELECT COUNT(*) FROM tree WHERE plugin = '$plugin_name';
"
}

tree_plugin_job_id() {
  local root_job_id="$1"
  local plugin_name="$2"
  sqlite3 "$DB_PATH" "
WITH RECURSIVE tree(id, plugin, created_at) AS (
  SELECT id, plugin, created_at
  FROM job_queue
  WHERE id = '$root_job_id'
  UNION ALL
  SELECT jq.id, jq.plugin, jq.created_at
  FROM job_queue jq
  JOIN tree ON jq.parent_job_id = tree.id
)
SELECT id
FROM tree
WHERE plugin = '$plugin_name'
ORDER BY created_at ASC
LIMIT 1;
"
}

job_status() {
  local job_id="$1"
  sqlite3 "$DB_PATH" "SELECT status FROM job_queue WHERE id = '$job_id';"
}

pipeline_instance_id_for_root() {
  local root_job_id="$1"
  sqlite3 "$DB_PATH" "
SELECT json_extract(ec.accumulated_json, '$.ductile.pipeline_instance_id')
FROM job_queue jq
JOIN event_context ec ON ec.id = jq.event_context_id
WHERE jq.id = '$root_job_id';
"
}

context_chain() {
  local pipeline_instance_id="$1"
  sqlite3 -separator '|' "$DB_PATH" "
SELECT step_id,
       COALESCE(json_extract(accumulated_json, '$.ductile.route_depth'), 0),
       COALESCE(json_extract(accumulated_json, '$.ductile.route_max_depth'), 0)
FROM event_context
WHERE json_extract(accumulated_json, '$.ductile.pipeline_instance_id') = '$pipeline_instance_id'
ORDER BY COALESCE(json_extract(accumulated_json, '$.ductile.route_depth'), 0), created_at;
"
}

fixture_log "starting ductile process"
"$ROOT_DIR/ductile" system start --config "$CONFIG_DIR" >"$ARTIFACT_DIR/ductile.log" 2>&1 &
PID=$!

ready=0
for _ in $(seq 1 40); do
  if curl -fsS http://127.0.0.1:18483/healthz >"$ARTIFACT_DIR/healthz.json" 2>/dev/null; then
    ready=1
    break
  fi
  sleep 0.25
done
if [[ "$ready" -ne 1 ]]; then
  fixture_fail "health endpoint did not become ready"
fi

fixture_log "triggering false branch"
FALSE_STATUS=$(curl -sS -o "$ARTIFACT_DIR/false-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18483/pipeline/conditional-with-route \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"run_processor":false,"subject":"false-branch","status":"ignored"}}')
if [[ "$FALSE_STATUS" != "202" ]]; then
  fixture_fail "expected 202 for false branch trigger, got $FALSE_STATUS"
fi
FALSE_ROOT_JOB_ID=$(jq -r '.job_id' "$ARTIFACT_DIR/false-response.json")
if [[ -z "$FALSE_ROOT_JOB_ID" || "$FALSE_ROOT_JOB_ID" == "null" ]]; then
  fixture_fail "false branch returned no job_id"
fi

wait_for_tree "$FALSE_ROOT_JOB_ID" "2" "false branch"

FALSE_SWITCH_COUNT=$(tree_plugin_count "$FALSE_ROOT_JOB_ID" "core.switch")
FALSE_PROCESSOR_COUNT=$(tree_plugin_count "$FALSE_ROOT_JOB_ID" "content_processor")
FALSE_SINK_COUNT=$(tree_plugin_count "$FALSE_ROOT_JOB_ID" "event_echo")
printf '%s\n' "$FALSE_SWITCH_COUNT" >"$ARTIFACT_DIR/false-switch-count.txt"
printf '%s\n' "$FALSE_PROCESSOR_COUNT" >"$ARTIFACT_DIR/false-processor-count.txt"
printf '%s\n' "$FALSE_SINK_COUNT" >"$ARTIFACT_DIR/false-sink-count.txt"
if [[ "$FALSE_SWITCH_COUNT" != "1" ]]; then
  fixture_fail "expected exactly 1 core.switch job in false branch, found $FALSE_SWITCH_COUNT"
fi
if [[ "$FALSE_PROCESSOR_COUNT" != "0" ]]; then
  fixture_fail "expected 0 content_processor jobs in false branch, found $FALSE_PROCESSOR_COUNT"
fi
if [[ "$FALSE_SINK_COUNT" != "1" ]]; then
  fixture_fail "expected 1 event_echo job in false branch, found $FALSE_SINK_COUNT"
fi

FALSE_ROOT_STATUS=$(job_status "$FALSE_ROOT_JOB_ID")
FALSE_SINK_JOB_ID=$(tree_plugin_job_id "$FALSE_ROOT_JOB_ID" "event_echo")
FALSE_SINK_STATUS=$(job_status "$FALSE_SINK_JOB_ID")
printf '%s\n' "$FALSE_ROOT_STATUS" >"$ARTIFACT_DIR/false-root-status.txt"
printf '%s\n' "$FALSE_SINK_STATUS" >"$ARTIFACT_DIR/false-sink-status.txt"
if [[ "$FALSE_ROOT_STATUS" != "succeeded" ]]; then
  fixture_fail "expected false-branch core.switch to succeed, got $FALSE_ROOT_STATUS"
fi
if [[ "$FALSE_SINK_STATUS" != "succeeded" ]]; then
  fixture_fail "expected false-branch event_echo to succeed, got $FALSE_SINK_STATUS"
fi

FALSE_INSTANCE_ID=$(pipeline_instance_id_for_root "$FALSE_ROOT_JOB_ID")
FALSE_CONTEXT_CHAIN=$(context_chain "$FALSE_INSTANCE_ID")
printf '%s\n' "$FALSE_CONTEXT_CHAIN" >"$ARTIFACT_DIR/false-context-chain.txt"
EXPECTED_FALSE_CONTEXT_CHAIN=$'|0|3\nmaybe_process__switch|1|3\nfinished|2|3'
if [[ "$FALSE_CONTEXT_CHAIN" != "$EXPECTED_FALSE_CONTEXT_CHAIN" ]]; then
  fixture_fail "unexpected false-branch context chain"
fi

fixture_log "triggering true branch"
TRUE_STATUS=$(curl -sS -o "$ARTIFACT_DIR/true-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18483/pipeline/conditional-with-route \
  -H 'Authorization: Bearer test-admin-token' \
  -H 'Content-Type: application/json' \
  --data '{"payload":{"run_processor":true,"subject":"true-branch","status":"approved"}}')
if [[ "$TRUE_STATUS" != "202" ]]; then
  fixture_fail "expected 202 for true branch trigger, got $TRUE_STATUS"
fi
TRUE_ROOT_JOB_ID=$(jq -r '.job_id' "$ARTIFACT_DIR/true-response.json")
if [[ -z "$TRUE_ROOT_JOB_ID" || "$TRUE_ROOT_JOB_ID" == "null" ]]; then
  fixture_fail "true branch returned no job_id"
fi

wait_for_tree "$TRUE_ROOT_JOB_ID" "3" "true branch"

TRUE_SWITCH_COUNT=$(tree_plugin_count "$TRUE_ROOT_JOB_ID" "core.switch")
TRUE_PROCESSOR_COUNT=$(tree_plugin_count "$TRUE_ROOT_JOB_ID" "content_processor")
TRUE_SINK_COUNT=$(tree_plugin_count "$TRUE_ROOT_JOB_ID" "event_echo")
printf '%s\n' "$TRUE_SWITCH_COUNT" >"$ARTIFACT_DIR/true-switch-count.txt"
printf '%s\n' "$TRUE_PROCESSOR_COUNT" >"$ARTIFACT_DIR/true-processor-count.txt"
printf '%s\n' "$TRUE_SINK_COUNT" >"$ARTIFACT_DIR/true-sink-count.txt"
if [[ "$TRUE_SWITCH_COUNT" != "1" ]]; then
  fixture_fail "expected exactly 1 core.switch job in true branch, found $TRUE_SWITCH_COUNT"
fi
if [[ "$TRUE_PROCESSOR_COUNT" != "1" ]]; then
  fixture_fail "expected 1 content_processor job in true branch, found $TRUE_PROCESSOR_COUNT"
fi
if [[ "$TRUE_SINK_COUNT" != "1" ]]; then
  fixture_fail "expected 1 event_echo job in true branch, found $TRUE_SINK_COUNT"
fi

TRUE_ROOT_STATUS=$(job_status "$TRUE_ROOT_JOB_ID")
TRUE_PROCESSOR_JOB_ID=$(tree_plugin_job_id "$TRUE_ROOT_JOB_ID" "content_processor")
TRUE_PROCESSOR_STATUS=$(job_status "$TRUE_PROCESSOR_JOB_ID")
TRUE_SINK_JOB_ID=$(tree_plugin_job_id "$TRUE_ROOT_JOB_ID" "event_echo")
TRUE_SINK_STATUS=$(job_status "$TRUE_SINK_JOB_ID")
printf '%s\n' "$TRUE_ROOT_STATUS" >"$ARTIFACT_DIR/true-root-status.txt"
printf '%s\n' "$TRUE_PROCESSOR_STATUS" >"$ARTIFACT_DIR/true-processor-status.txt"
printf '%s\n' "$TRUE_SINK_STATUS" >"$ARTIFACT_DIR/true-sink-status.txt"
if [[ "$TRUE_ROOT_STATUS" != "succeeded" ]]; then
  fixture_fail "expected true-branch core.switch to succeed, got $TRUE_ROOT_STATUS"
fi
if [[ "$TRUE_PROCESSOR_STATUS" != "succeeded" ]]; then
  fixture_fail "expected true-branch content_processor to succeed, got $TRUE_PROCESSOR_STATUS"
fi
if [[ "$TRUE_SINK_STATUS" != "succeeded" ]]; then
  fixture_fail "expected true-branch event_echo to succeed, got $TRUE_SINK_STATUS"
fi

TRUE_INSTANCE_ID=$(pipeline_instance_id_for_root "$TRUE_ROOT_JOB_ID")
TRUE_CONTEXT_CHAIN=$(context_chain "$TRUE_INSTANCE_ID")
printf '%s\n' "$TRUE_CONTEXT_CHAIN" >"$ARTIFACT_DIR/true-context-chain.txt"
EXPECTED_TRUE_CONTEXT_CHAIN=$'|0|3\nmaybe_process__switch|1|3\nmaybe_process|2|3\nfinished|3|3'
if [[ "$TRUE_CONTEXT_CHAIN" != "$EXPECTED_TRUE_CONTEXT_CHAIN" ]]; then
  fixture_fail "unexpected true-branch context chain"
fi

# As of Sprint 18 the core no longer provisions a workspace; the
# content_processor plugin runs in its configured working_dir
# (plugins.yaml: content_processor.config.working_dir = ./state) and
# writes with-proof.json there.
WITH_PROOF_FILE="$STATE_DIR/with-proof.json"
fixture_capture_file "$WITH_PROOF_FILE" true-with-proof.json
if [[ ! -f "$WITH_PROOF_FILE" ]]; then
  fixture_fail "with-proof.json not found at $WITH_PROOF_FILE"
fi

WITH_CONTENT=$(jq -r '.content' "$WITH_PROOF_FILE")
WITH_MESSAGE=$(jq -r '.message' "$WITH_PROOF_FILE")
WITH_STATUS=$(jq -r '.status' "$WITH_PROOF_FILE")
WITH_SUBJECT=$(jq -r '.subject' "$WITH_PROOF_FILE")
printf '%s\n' "$WITH_CONTENT" >"$ARTIFACT_DIR/with-content.txt"
printf '%s\n' "$WITH_MESSAGE" >"$ARTIFACT_DIR/with-message.txt"
printf '%s\n' "$WITH_STATUS" >"$ARTIFACT_DIR/with-status.txt"
printf '%s\n' "$WITH_SUBJECT" >"$ARTIFACT_DIR/with-subject.txt"

if [[ "$WITH_CONTENT" != "branch true for true-branch" ]]; then
  fixture_fail "unexpected with content: $WITH_CONTENT"
fi
if [[ "$WITH_MESSAGE" != "with transformed true-branch" ]]; then
  fixture_fail "unexpected with message: $WITH_MESSAGE"
fi
if [[ "$WITH_STATUS" != "approved" ]]; then
  fixture_fail "unexpected with status: $WITH_STATUS"
fi
if [[ "$WITH_SUBJECT" != "true-branch" ]]; then
  fixture_fail "unexpected retained subject: $WITH_SUBJECT"
fi

fixture_log "success"
