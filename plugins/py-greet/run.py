#!/usr/bin/env python3
# py-greet: Python example plugin for Ductile Gateway
# Demonstrates protocol v2 JSON I/O over stdin/stdout.

import json
import sys
from datetime import datetime, timezone


def poll(req: dict) -> dict:
    greeting = req.get("config", {}).get("greeting", "Hello")
    name = req.get("config", {}).get("name", "World")
    now = datetime.now(timezone.utc).isoformat()

    message = f"{greeting}, {name}!"
    return {
        "status": "ok",
        "result": message,
        "events": [],
        "state_updates": {
            "last_run": now,
            "last_greeting": message,
        },
        "logs": [
            {"level": "info", "message": f"{message} (job: {req.get('job_id')})"},
        ],
    }


def health(req: dict) -> dict:
    return {
        "status": "ok",
        "result": "healthy",
        "logs": [{"level": "info", "message": "healthy"}],
    }


def main():
    request = json.load(sys.stdin)
    command = request.get("command")

    if command == "poll":
        response = poll(request)
    elif command == "health":
        response = health(request)
    else:
        response = {
            "status": "error",
            "error": f"unknown command: {command}",
            "retry": False,
            "logs": [{"level": "error", "message": f"unknown command: {command}"}],
        }

    sys.stdout.write(json.dumps(response))


if __name__ == "__main__":
    main()
