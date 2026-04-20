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
DUCTILE_PID=""
SERVER_PID=""
rm -rf "$STATE_DIR"
mkdir -p "$STATE_DIR"
SCENARIO_LOG="$ARTIFACT_DIR/scenario.log"
exec > >(tee "$SCENARIO_LOG") 2>&1

cleanup() {
  fixture_capture_file "$DB_PATH" state.db
  fixture_capture_tree "$STATE_DIR" state-dir
  if [[ -n "$DUCTILE_PID" ]]; then
    kill "$DUCTILE_PID" 2>/dev/null || true
    wait "$DUCTILE_PID" 2>/dev/null || true
  fi
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Start a local HTTP server that the fetch plugin will call
#
# /html     — returns text/html (exercises markdown fallback path, since plugin
#             is configured with output_format: markdown)
# /markdown — returns text/markdown when Accept header requests it
# /redirect — 302 → /html
# ---------------------------------------------------------------------------
fixture_log "starting local HTTP test server"
python3 - <<'PYEOF' &
import http.server

class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        accept = self.headers.get("Accept", "")
        if self.path == "/html":
            body = b"<html><body><h1>Hello</h1><p>World</p></body></html>"
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        elif self.path == "/markdown":
            if "text/markdown" in accept:
                body = b"# Hello\n\nThis is **markdown** content."
                self.send_response(200)
                self.send_header("Content-Type", "text/markdown; charset=utf-8")
                self.send_header("Content-Length", str(len(body)))
                self.send_header("x-markdown-tokens", "9")
                self.end_headers()
                self.wfile.write(body)
            else:
                body = b"<html><body><h1>Hello</h1></body></html>"
                self.send_response(200)
                self.send_header("Content-Type", "text/html; charset=utf-8")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
        elif self.path == "/redirect":
            self.send_response(302)
            self.send_header("Location", "http://127.0.0.1:19191/html")
            self.end_headers()
        else:
            self.send_response(404)
            self.end_headers()

http.server.HTTPServer(("127.0.0.1", 19191), H).serve_forever()
PYEOF
SERVER_PID=$!

for _ in $(seq 1 20); do
  curl -fsS http://127.0.0.1:19191/html >/dev/null 2>&1 && break
  sleep 0.1
done
curl -fsS http://127.0.0.1:19191/html >/dev/null || fixture_fail "test HTTP server did not start"
fixture_log "test server ready on :19191"

# ---------------------------------------------------------------------------
# Start ductile (plugin configured with output_format: markdown)
# ---------------------------------------------------------------------------
fixture_log "starting ductile process"
"$ROOT_DIR/ductile" system start --config "$CONFIG_DIR" >"$ARTIFACT_DIR/ductile.log" 2>&1 &
DUCTILE_PID=$!

ready=0
for _ in $(seq 1 40); do
  if curl -fsS http://127.0.0.1:18282/healthz >"$ARTIFACT_DIR/healthz.json" 2>/dev/null; then
    ready=1; break
  fi
  sleep 0.25
done
[[ "$ready" -eq 1 ]] || fixture_fail "ductile health endpoint did not become ready"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
trigger() {
  local label="$1" payload="$2"
  local out="$ARTIFACT_DIR/${label}-response.json"
  local code
  code=$(curl -sS -o "$out" -w '%{http_code}' -X POST \
    http://127.0.0.1:18282/plugin/fetch/handle \
    -H 'Authorization: Bearer test-admin-token' \
    -H 'Content-Type: application/json' \
    --data "$payload")
  [[ "$code" == "202" ]] || fixture_fail "$label: expected 202, got $code"
  jq -r '.job_id' "$out"
}

wait_for_job() {
  local job_id="$1"
  for _ in $(seq 1 40); do
    if [[ -f "$DB_PATH" ]]; then
      local s
      s=$(sqlite3 "$DB_PATH" \
        "SELECT status FROM job_log WHERE id LIKE '${job_id}%' LIMIT 1;" 2>/dev/null || true)
      [[ -n "$s" ]] && { echo "$s"; return 0; }
    fi
    sleep 0.25
  done
  echo "timeout"
}

db_result_field() {
  local job_id="$1" path="$2"
  sqlite3 "$DB_PATH" \
    "SELECT json_extract(result, '$path') FROM job_log WHERE id LIKE '${job_id}%' LIMIT 1;" \
    2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Scenario 1: native markdown — server honours Accept: text/markdown
# ---------------------------------------------------------------------------
fixture_log "scenario: markdown (server returns text/markdown)"
JOB=$(trigger "markdown-native" '{"payload":{"url":"http://127.0.0.1:19191/markdown"}}')
STATUS=$(wait_for_job "$JOB")
[[ "$STATUS" == "succeeded" ]] || fixture_fail "markdown-native: expected succeeded, got $STATUS"
FMT=$(db_result_field "$JOB" '$.events[0].payload.output_format')
[[ "$FMT" == "markdown" ]] || fixture_fail "markdown-native: expected output_format=markdown, got '$FMT'"
TOKENS=$(db_result_field "$JOB" '$.events[0].payload.markdown_tokens')
[[ "$TOKENS" == "9" ]] || fixture_fail "markdown-native: expected markdown_tokens=9, got '$TOKENS'"
fixture_log "markdown-native: OK (format=$FMT, tokens=$TOKENS)"

# ---------------------------------------------------------------------------
# Scenario 2: markdown fallback — server returns HTML, plugin falls back gracefully
# ---------------------------------------------------------------------------
fixture_log "scenario: markdown fallback (server returns text/html)"
JOB=$(trigger "markdown-fallback" '{"payload":{"url":"http://127.0.0.1:19191/html"}}')
STATUS=$(wait_for_job "$JOB")
[[ "$STATUS" == "succeeded" ]] || fixture_fail "markdown-fallback: expected succeeded, got $STATUS"
FMT=$(db_result_field "$JOB" '$.events[0].payload.output_format')
[[ "$FMT" == "html" ]] || fixture_fail "markdown-fallback: expected output_format=html, got '$FMT'"
TOKEN_FIELD=$(db_result_field "$JOB" '$.events[0].payload.markdown_tokens')
[[ -z "$TOKEN_FIELD" ]] || fixture_fail "markdown-fallback: markdown_tokens should be absent, got '$TOKEN_FIELD'"
fixture_log "markdown-fallback: OK (fell back to $FMT, no token count)"

# ---------------------------------------------------------------------------
# Scenario 3: redirect followed (default behaviour)
# ---------------------------------------------------------------------------
fixture_log "scenario: redirect followed"
JOB=$(trigger "redirect" '{"payload":{"url":"http://127.0.0.1:19191/redirect"}}')
STATUS=$(wait_for_job "$JOB")
[[ "$STATUS" == "succeeded" ]] || fixture_fail "redirect: expected succeeded, got $STATUS"
FINAL=$(db_result_field "$JOB" '$.events[0].payload.final_url')
[[ "$FINAL" == *"/html"* ]] || fixture_fail "redirect: expected final_url ending in /html, got '$FINAL'"
fixture_log "redirect followed: OK (final_url=$FINAL)"

# ---------------------------------------------------------------------------
# Scenario 4: missing URL → job fails non-retryably
# ---------------------------------------------------------------------------
fixture_log "scenario: missing url"
JOB=$(trigger "missing-url" '{"payload":{}}')
STATUS=$(wait_for_job "$JOB")
[[ "$STATUS" == "failed" ]] || fixture_fail "missing-url: expected failed, got $STATUS"
fixture_log "missing-url: OK ($STATUS)"

fixture_log "success"
