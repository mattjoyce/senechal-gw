#!/usr/bin/env bash
# emit_event — fixture plugin for the pipeline-level-if scenario.
# Reads a JSON request from stdin and emits one event whose type +
# base payload come from config.event_type / config.event_payload, with
# request payload values merged on top so the test can vary kind per
# invocation.
set -euo pipefail
request="$(cat)"
python3 - "$request" <<'PY'
import json, sys
req = json.loads(sys.argv[1])
cfg = req.get("config") or {}
event_type = cfg.get("event_type", "fixture.event")
base = dict(cfg.get("event_payload") or {})
incoming = dict(req.get("event", {}).get("payload") or {})
base.update(incoming)
out = {
    "status": "ok",
    "result": f"emitted {event_type}",
    "events": [{"type": event_type, "payload": base}],
}
json.dump(out, sys.stdout)
PY
