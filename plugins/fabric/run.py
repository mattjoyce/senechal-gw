#!/usr/bin/env python3
"""Fabric AI wrapper plugin for Senechal Gateway.

Wraps the fabric CLI tool to process text through predefined patterns.
Protocol v1: reads JSON from stdin, writes JSON to stdout.
"""
import json
import subprocess
import sys
from datetime import datetime, timezone


def execute_command(config, state, event):
    payload = event.get("payload", {})

    text = payload.get("text")
    if not text:
        return error_response("Missing 'text' field in event payload")

    pattern = payload.get("pattern")
    if not pattern:
        return error_response("Missing 'pattern' field in event payload")

    model = payload.get("model") or config.get("FABRIC_DEFAULT_MODEL")
    fabric_bin = config.get("FABRIC_BIN_PATH", "fabric")

    cmd = [fabric_bin, "--pattern", pattern]
    if model:
        cmd.extend(["--model", model])

    try:
        result = subprocess.run(
            cmd,
            input=text,
            capture_output=True,
            text=True,
            timeout=120,
        )
    except FileNotFoundError:
        return error_response(
            f"Fabric binary not found at '{fabric_bin}'. "
            "Install with: go install github.com/danielmiessler/fabric@latest",
            retry=False,
        )
    except subprocess.TimeoutExpired:
        return error_response("Fabric execution timed out after 120s", retry=True)

    if result.returncode != 0:
        stderr = result.stderr.strip()
        return error_response(f"Fabric failed (exit {result.returncode}): {stderr}", retry=True)

    output = result.stdout.strip()
    executions_count = state.get("executions_count", 0) + 1

    return {
        "status": "ok",
        "events": [
            {
                "type": "fabric.completed",
                "payload": {
                    "result": output,
                    "pattern": pattern,
                    "model": model or "default",
                    "input_length": len(text),
                    "output_length": len(output),
                },
            }
        ],
        "state_updates": {
            "last_run": datetime.now(timezone.utc).isoformat(),
            "executions_count": executions_count,
            "last_pattern": pattern,
        },
        "logs": [
            {"level": "info", "message": f"Executed fabric pattern: {pattern}"},
        ],
    }


def health_command(config):
    fabric_bin = config.get("FABRIC_BIN_PATH", "fabric")

    try:
        result = subprocess.run(
            [fabric_bin, "--help"],
            capture_output=True,
            text=True,
            timeout=10,
        )
    except FileNotFoundError:
        return error_response(
            f"Fabric binary not found at '{fabric_bin}'. "
            "Install with: go install github.com/danielmiessler/fabric@latest",
            retry=False,
        )
    except subprocess.TimeoutExpired:
        return error_response("Fabric health check timed out", retry=False)

    if result.returncode != 0:
        return error_response(f"Fabric --help failed: {result.stderr.strip()}", retry=False)

    # Count available patterns
    pattern_count = 0
    try:
        list_result = subprocess.run(
            [fabric_bin, "--listpatterns"],
            capture_output=True,
            text=True,
            timeout=10,
        )
        if list_result.returncode == 0:
            pattern_count = len([line for line in list_result.stdout.strip().splitlines() if line.strip()])
    except (FileNotFoundError, subprocess.TimeoutExpired):
        pass

    return {
        "status": "ok",
        "state_updates": {
            "available_patterns": pattern_count,
            "last_health_check": datetime.now(timezone.utc).isoformat(),
        },
        "logs": [
            {"level": "info", "message": f"Fabric healthy, {pattern_count} patterns available"},
        ],
    }


def error_response(message, retry=False):
    return {
        "status": "error",
        "error": message,
        "retry": retry,
        "logs": [{"level": "error", "message": message}],
    }


def main():
    request = json.load(sys.stdin)
    command = request.get("command", "")
    config = request.get("config", {})
    state = request.get("state", {})
    event = request.get("event", {})

    if command == "execute":
        response = execute_command(config, state, event)
    elif command == "health":
        response = health_command(config)
    else:
        response = error_response(f"Unknown command: {command}")

    json.dump(response, sys.stdout)


if __name__ == "__main__":
    main()
