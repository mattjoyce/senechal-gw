#!/usr/bin/env python3
"""Fabric AI wrapper plugin for Senechal Gateway.

Wraps the fabric CLI tool to process text through predefined patterns.
Protocol v1: reads JSON from stdin, writes JSON to stdout.
"""
import json
import subprocess
import sys
from datetime import datetime, timezone


def poll_command(config, state):
    """Poll command - for scheduled execution (not implemented yet)"""
    return {
        "status": "ok",
        "state_updates": {
            "last_poll": datetime.now(timezone.utc).isoformat(),
        },
        "logs": [
            {"level": "info", "message": "Fabric poll command - no scheduled actions configured"},
        ],
    }


def handle_command(config, state, event):
    """Handle command - processes events with fabric patterns"""
    payload = event.get("payload", {})

    text = payload.get("text")
    if not text:
        return error_response("Missing 'text' field in event payload")

    pattern = payload.get("pattern") or config.get("FABRIC_DEFAULT_PATTERN")
    prompt = payload.get("prompt") or config.get("FABRIC_DEFAULT_PROMPT")

    model = payload.get("model") or config.get("FABRIC_DEFAULT_MODEL")
    fabric_bin = config.get("FABRIC_BIN_PATH", "fabric")

    # Support two modes:
    # 1) pattern mode: fabric --pattern <preset> (existing behavior)
    # 2) prompt mode: no pattern required; prompt is prepended to text input
    cmd = [fabric_bin]
    if pattern:
        cmd.extend(["--pattern", pattern])
    if model:
        cmd.extend(["--model", model])

    input_text = text
    if prompt:
        input_text = f"{prompt}\n\n{text}"

    try:
        result = subprocess.run(
            cmd,
            input=input_text,
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

    # Build event payload with fabric results
    event_payload = {
        "result": output,
        "pattern": pattern or "",
        "prompt": prompt or "",
        "model": model or "default",
        "input_length": len(text),
        "output_length": len(output),
    }

    # Propagate pipeline context fields for downstream steps
    # (e.g., output_dir, output_path, filename from upstream plugins)
    for field in ["output_dir", "output_path", "filename", "file_path"]:
        if field in payload:
            event_payload[field] = payload[field]

    return {
        "status": "ok",
        "events": [
            {
                "type": "fabric.completed",
                "payload": event_payload,
            }
        ],
        "state_updates": {
            "last_run": datetime.now(timezone.utc).isoformat(),
            "executions_count": executions_count,
            "last_pattern": pattern or "",
            "last_prompt": prompt or "",
        },
        "logs": [
            {
                "level": "info",
                "message": (
                    f"Executed fabric pattern: {pattern}"
                    if pattern else
                    "Executed fabric with prompt/no-pattern mode"
                ),
            },
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

    if command == "poll":
        response = poll_command(config, state)
    elif command == "handle":
        response = handle_command(config, state, event)
    elif command == "health":
        response = health_command(config)
    else:
        response = error_response(f"Unknown command: {command}")

    json.dump(response, sys.stdout)


if __name__ == "__main__":
    main()
