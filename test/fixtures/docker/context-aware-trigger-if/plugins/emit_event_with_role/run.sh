#!/usr/bin/env bash
# emit_event_with_role — fixture plugin that emits an event with role in payload.
# Used to test context.* availability in trigger-level if: predicates.
set -euo pipefail
request="$(cat)"
python3 - "$request" <<'PY'
import json, sys
req = json.loads(sys.argv[1])
cfg = req.get("config") or {}
event_type = cfg.get("event_type", "test.downstream.check")
role = cfg.get("role", "admin")
out = {
    "status": "ok",
    "result": f"emitted {event_type} with role={role}",
    "events": [{"type": event_type, "payload": {"role": role}}],
}
json.dump(out, sys.stdout)
PY
