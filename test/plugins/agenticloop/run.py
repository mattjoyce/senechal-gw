#!/usr/bin/env python3
"""AgenticLoop wake plugin for ductile (protocol v2).

Calls AgenticLoop's POST /v1/wake endpoint to enqueue an agent run.

Payload fields (handle command):
  goal        (required) - The agent's goal string
  wake_id     (optional) - Idempotency key; omit for a fresh run
  context     (optional) - Dict of static context passed to the agent
  constraints (optional) - Dict with max_loops (int) and/or deadline (str e.g. "5m")
"""

import json
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone
from typing import Any, Dict


def _now() -> str:
    return datetime.now(timezone.utc).isoformat()


def error_response(message: str, retry: bool = False) -> Dict[str, Any]:
    return {
        "status": "error",
        "error": message,
        "retry": retry,
        "logs": [{"level": "error", "message": message}],
    }


def handle_command(config: Dict[str, Any], event: Dict[str, Any]) -> Dict[str, Any]:
    base_url = (config.get("base_url") or "").rstrip("/")
    token = config.get("token") or ""

    payload = event.get("payload", {})
    goal = (payload.get("goal") or "").strip()
    if not goal:
        return error_response("payload.goal is required", retry=False)

    wake_id = payload.get("wake_id") or None
    context = payload.get("context") or {}
    constraints = payload.get("constraints") or {}

    body: Dict[str, Any] = {"goal": goal, "context": context, "constraints": constraints}
    if wake_id:
        body["wake_id"] = wake_id

    url = f"{base_url}/v1/wake"
    raw = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=raw,
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        },
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            resp_body = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        body_text = e.read().decode("utf-8", errors="replace")
        return error_response(
            f"AgenticLoop wake failed HTTP {e.code}: {body_text}",
            retry=(e.code >= 500),
        )
    except urllib.error.URLError as e:
        return error_response(f"AgenticLoop unreachable: {e.reason}", retry=True)
    except Exception as e:
        return error_response(str(e), retry=False)

    run_id = resp_body.get("run_id", "")
    status = resp_body.get("status", "")
    existing = resp_body.get("existing", False)

    return {
        "status": "ok",
        "events": [
            {
                "type": "agenticloop.woke",
                "payload": {
                    "run_id": run_id,
                    "wake_id": wake_id,
                    "run_status": status,
                    "existing": existing,
                    "goal": goal,
                },
            }
        ],
        "state_updates": {
            "last_run": _now(),
            "last_run_id": run_id,
        },
        "logs": [
            {
                "level": "info",
                "message": (
                    f"Woke AgenticLoop run_id={run_id} status={status}"
                    f"{' (existing)' if existing else ''}"
                ),
            }
        ],
    }


def poll_command(state: Dict[str, Any]) -> Dict[str, Any]:
    return {
        "status": "ok",
        "state_updates": {"last_poll": _now()},
        "logs": [{"level": "info", "message": "agenticloop poll (no-op, event-driven)"}],
    }


def health_command(config: Dict[str, Any]) -> Dict[str, Any]:
    base_url = (config.get("base_url") or "").rstrip("/")
    url = f"{base_url}/healthz"
    try:
        with urllib.request.urlopen(url, timeout=5) as resp:
            body = json.loads(resp.read().decode("utf-8"))
        svc_status = body.get("status", "unknown")
        return {
            "status": "ok",
            "state_updates": {"last_health_check": _now()},
            "logs": [{"level": "info", "message": f"AgenticLoop health: {svc_status}"}],
        }
    except Exception as e:
        return error_response(f"AgenticLoop health check failed: {e}", retry=True)


def main() -> None:
    request = json.load(sys.stdin)
    command = request.get("command", "poll")
    config = request.get("config", {})
    state = request.get("state", {})
    event = request.get("event", {})

    if command == "poll":
        response = poll_command(state)
    elif command == "handle":
        response = handle_command(config, event)
    elif command == "health":
        response = health_command(config)
    else:
        response = error_response(f"Unknown command: {command}")

    json.dump(response, sys.stdout)


if __name__ == "__main__":
    main()
