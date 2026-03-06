#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:?}"
FIXTURE_NAME="${FIXTURE_NAME:?}"
ARTIFACT_ROOT="${ARTIFACT_ROOT:?}"
FIXTURE_DIR="$ROOT_DIR/test/fixtures/docker/$FIXTURE_NAME"
CONFIG_DIR="$FIXTURE_DIR/config"
STATE_DIR="$FIXTURE_DIR/state"
ARTIFACT_DIR="$ARTIFACT_ROOT/$FIXTURE_NAME"
PID=""

mkdir -p "$STATE_DIR" "$ARTIFACT_DIR"
SCENARIO_LOG="$ARTIFACT_DIR/scenario.log"
exec > >(tee "$SCENARIO_LOG") 2>&1

echo "[fixture:$FIXTURE_NAME] starting ductile process"
cleanup() {
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

"$ROOT_DIR/ductile" system start --config "$CONFIG_DIR" >"$ARTIFACT_DIR/ductile.log" 2>&1 &
PID=$!

for _ in $(seq 1 40); do
  if curl -fsS http://127.0.0.1:18081/healthz >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done
curl -fsS http://127.0.0.1:18081/healthz >"$ARTIFACT_DIR/healthz.json"

echo "[fixture:$FIXTURE_NAME] sending valid webhook"
BODY='{"event":"push"}'
SIG=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac 'test-secret' -hex | awk '{print $2}')
VALID_STATUS=$(curl -sS -o "$ARTIFACT_DIR/valid-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18091/webhook/github \
  -H "X-Hub-Signature-256: sha256=$SIG" \
  -H 'Content-Type: application/json' \
  --data "$BODY")
if [[ "$VALID_STATUS" != "202" ]]; then
  echo "expected 202 for valid webhook, got $VALID_STATUS"
  exit 1
fi

echo "[fixture:$FIXTURE_NAME] sending invalid webhook"
INVALID_STATUS=$(curl -sS -o "$ARTIFACT_DIR/invalid-response.json" -w '%{http_code}' -X POST \
  http://127.0.0.1:18091/webhook/github \
  -H 'X-Hub-Signature-256: sha256=deadbeef' \
  -H 'Content-Type: application/json' \
  --data "$BODY")
if [[ "$INVALID_STATUS" != "403" ]]; then
  echo "expected 403 for invalid webhook, got $INVALID_STATUS"
  exit 1
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
  echo "expected queued webhook job, found $COUNT"
  exit 1
fi

cp "$DB_PATH" "$ARTIFACT_DIR/state.db"
echo "[fixture:$FIXTURE_NAME] success"
