#!/usr/bin/env python3
"""Fabric AI wrapper plugin for Ductile Gateway.

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

    # Extract input parameters
    text = payload.get("text")
    url = payload.get("url")
    youtube_url = payload.get("youtube_url")

    # Only use default pattern if pattern not explicitly provided AND not in prompt-only mode
    # Prompt-only mode: prompt provided without text/url/youtube_url and without explicit pattern
    pattern = payload.get("pattern")
    if pattern is None and not (payload.get("prompt") and not any([text, url, youtube_url])):
        pattern = config.get("FABRIC_DEFAULT_PATTERN")

    prompt = payload.get("prompt") or config.get("FABRIC_DEFAULT_PROMPT")
    model = payload.get("model") or config.get("FABRIC_DEFAULT_MODEL")
    fabric_bin = config.get("FABRIC_BIN_PATH", "fabric")

    # Validate input: need at least one of text, url, youtube_url, or prompt
    if not any([text, url, youtube_url, prompt]):
        return error_response("Missing input: provide 'text', 'url', 'youtube_url', or 'prompt'")

    # Build fabric command
    # Support multiple modes:
    # 1) pattern mode: fabric --pattern <preset>
    # 2) prompt mode: no pattern required; prompt is prepended to text input
    # 3) URL mode: fabric --scrape_url=<url>
    # 4) YouTube mode: fabric --youtube=<url>
    cmd = [fabric_bin]

    if pattern:
        cmd.extend(["--pattern", pattern])
    if model:
        cmd.extend(["--model", model])
    if url:
        cmd.extend(["--scrape_url", url])
    if youtube_url:
        cmd.extend(["--youtube", youtube_url])

    # Prepare input text
    # If using URL/YouTube, fabric fetches content automatically
    # If using text, optionally prepend prompt
    input_text = ""
    if text:
        input_text = text
        if prompt:
            input_text = f"{prompt}\n\n{text}"
    elif prompt and not (url or youtube_url):
        # One-shot question mode: just a prompt with no text/URL
        input_text = prompt
    elif prompt and (url or youtube_url):
        # URL/YouTube with prompt: fabric will fetch content, we prepend prompt
        input_text = prompt

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
        "url": url or "",
        "youtube_url": youtube_url or "",
        "input_length": len(text) if text else 0,
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
                "message": build_log_message(pattern, url, youtube_url, prompt),
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


def build_log_message(pattern, url, youtube_url, prompt):
    """Build descriptive log message based on execution mode"""
    parts = []
    if pattern:
        parts.append(f"pattern={pattern}")
    if url:
        parts.append(f"url={url[:50]}...")
    if youtube_url:
        parts.append(f"youtube={youtube_url[:50]}...")
    if prompt and not (url or youtube_url):
        parts.append("prompt-mode")

    mode = ", ".join(parts) if parts else "text-mode"
    return f"Executed fabric ({mode})"


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
