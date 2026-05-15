#!/usr/bin/env bash
# Ductile "tsnotify" plugin (protocol 2) — fire-and-forget Tree
# Signal notifier.
#
# value/state/identity: a Tree Signal message is a value (an
# immutable event at a point in time); the dashboard is external
# state we do not own; the channel is identity. The plugin holds
# no state.
#
# Armstrong: telemetry must NEVER fail the work it reports on. A
# delivery failure is bounded (curl --max-time), logged, and
# returns status:ok with a warn log — never status:error, never a
# retry. Reporting that the dashboard is down by failing the job
# would be the footgun.
#
# protocol-2 `result` is string-typed — keep it a string.
set -euo pipefail

req="$(cat)"
if ! printf '%s' "$req" | jq -e . >/dev/null 2>&1; then
  jq -n '{status:"ok", result:"tsnotify: invalid request JSON (ignored)", events:[], logs:[{level:"warn",message:"invalid request JSON"}]}'
  exit 0
fi
jqr() { printf '%s' "$req" | jq -r "$1" 2>/dev/null || true; }

command_val="$(jqr '.command // "notify"')"
ts_url="$(jqr '.config.ts_url // "http://192.168.20.4:8013/v1/messages"')"
ts_base="${ts_url%/v1/messages}"
channel="$(jqr '.payload.channel // (.config.channel // "pentest.testbus")')"
severity="$(jqr '.payload.severity // (.config.severity // "info")')"
message="$(jqr '.payload.message // (.payload.payload // "")')"

case "$severity" in debug|info|warn|error) ;; *) severity="info" ;; esac

ok()   { jq -n --arg r "$1" '{status:"ok", result:$r, events:[], logs:[{level:"info",message:$r}]}'; }
warn() { jq -n --arg r "$1" '{status:"ok", result:$r, events:[], logs:[{level:"warn",message:$r}]}'; }

if [[ "$command_val" == "health" ]]; then
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$ts_base/healthz" 2>/dev/null || echo 000)"
  if [[ "$code" == "200" ]]; then ok "tree-signal healthy ($ts_base)"; else warn "tree-signal /healthz returned $code ($ts_base)"; fi
  exit 0
fi

if [[ "$command_val" != "notify" ]]; then
  warn "tsnotify: unknown command '$command_val' (ignored)"
  exit 0
fi

[[ -n "$message" ]] || { warn "tsnotify: empty message (nothing posted)"; exit 0; }

body="$(jq -n --arg c "$channel" --arg p "$message" --arg s "$severity" \
  '{channel:$c, payload:$p, severity:$s}')"

set +e
resp="$(curl -s -m 6 -w '\n%{http_code}' -X POST "$ts_url" \
  -H 'Content-Type: application/json' --data-binary "$body" 2>/dev/null)"
cstat=$?
set -e
code="$(printf '%s' "$resp" | tail -n1)"

if [[ $cstat -eq 0 && "$code" == "202" ]]; then
  ok "tsnotify: posted to $channel ($severity) -> 202"
else
  # Bounded + logged + non-fatal: the job that called us still wins.
  warn "tsnotify: delivery failed (curl=$cstat http=$code) channel=$channel — telemetry dropped, job not affected"
fi
exit 0
