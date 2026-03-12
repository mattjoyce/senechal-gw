#!/usr/bin/env python3
"""Discord notify plugin for Ductile Gateway (protocol v2).

Posts messages to a Discord channel via incoming webhook.
Reads message content from payload fields: message, content, result, or title.
Supports message_template config key with {field.path} dot-notation substitution.
Protocol v2: reads JSON from stdin, writes JSON to stdout.
"""
from __future__ import annotations

import json
import re
import sys
import urllib.error
import urllib.request
from typing import Any, Dict, List, Optional


def resolve_template(template: str, payload: Dict[str, Any]) -> str:
    """Render {field.path} placeholders using dot-notation lookup in payload.

    Missing or non-string-convertible fields render as empty string.
    Returns the rendered string (may be empty if all fields missing).
    """
    def replacer(match: re.Match) -> str:
        path = match.group(1)
        value: Any = payload
        for part in path.split("."):
            if isinstance(value, dict):
                value = value.get(part)
            else:
                value = None
                break
        return str(value) if value is not None else ""

    return re.sub(r"\{([^}]+)\}", replacer, template)


def pick(payload: Dict[str, Any], context: Dict[str, Any], *keys: str, default: Any = None) -> Any:
    for key in keys:
        if key in payload and payload[key] not in (None, ""):
            return payload[key]
        if key in context and context[key] not in (None, ""):
            return context[key]
    return default


def error_response(message: str, retry: bool = False) -> Dict[str, Any]:
    return {
        "status": "error",
        "error": message,
        "retry": retry,
        "logs": [{"level": "error", "message": message}],
    }


def ok_response(log_message: str) -> Dict[str, Any]:
    return {
        "status": "ok",
        "result": log_message,
        "logs": [{"level": "info", "message": log_message}],
    }


def post_to_discord(webhook_url: str, discord_payload: Dict[str, Any], timeout: int) -> None:
    """POST discord_payload to webhook_url. Raises on failure."""
    data = json.dumps(discord_payload).encode("utf-8")
    req = urllib.request.Request(
        webhook_url,
        data=data,
        headers={
            "Content-Type": "application/json",
            "User-Agent": "ductile-discord-notify/1 (https://github.com/mattjoyce/ductile)",
        },
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:  # noqa: S310
        if resp.status not in (200, 204):
            raise RuntimeError(f"Discord returned HTTP {resp.status}")


def handle_command(
    config: Dict[str, Any],
    payload: Dict[str, Any],
    context: Dict[str, Any],
) -> Dict[str, Any]:
    webhook_url = str(config.get("webhook_url") or "").strip()
    if not webhook_url:
        return error_response(
            "No webhook_url configured. Add to plugin config.", retry=False
        )

    content = pick(payload, context, "message", "content", default=None)
    title = pick(payload, context, "title", default=None)

    if not content and not title:
        # Try message_template before falling back to result/default_message.
        # This prevents internal pipeline result strings from leaking into Discord
        # when the caller has explicitly configured a template.
        template = str(config.get("message_template") or "").strip()
        if template:
            rendered = resolve_template(template, payload).strip()
            if rendered:
                content = rendered

    if not content and not title:
        # Check config-level default_message before falling back to context result.
        # default_message is an explicit config intent and should not be shadowed by
        # implicit pipeline result strings leaking through context.
        default_msg = str(config.get("default_message") or "").strip()
        if default_msg:
            content = default_msg

    if not content and not title:
        content = pick(payload, context, "result", default=None)

    if not content and not title:
        return error_response(
            "No message content found in payload (tried: message, content, result, title, message_template, default_message)",
            retry=False,
        )

    # Combine title + body, or use whichever is present
    if title and content:
        text = f"**{title}**\n{content}"
    else:
        text = str(title or content)

    # Discord hard limit is 2000 characters
    if len(text) > 2000:
        text = text[:1997] + "..."

    username = (
        pick(payload, context, "username", default=None)
        or str(config.get("default_username") or "Ductile").strip()
    )
    avatar_url = str(config.get("default_avatar_url") or "").strip() or None

    discord_payload: Dict[str, Any] = {"content": text, "username": username}
    if avatar_url:
        discord_payload["avatar_url"] = avatar_url

    timeout = int(config.get("request_timeout_seconds", 10))

    try:
        post_to_discord(webhook_url, discord_payload, timeout)
    except urllib.error.HTTPError as e:
        return error_response(
            f"Discord webhook HTTP {e.code}: {e.reason}",
            retry=e.code >= 500,
        )
    except urllib.error.URLError as e:
        return error_response(
            f"Discord webhook network error: {e.reason}",
            retry=True,
        )
    except Exception as e:  # noqa: BLE001
        return error_response(
            f"Failed to post to Discord: {type(e).__name__}: {e}",
            retry=True,
        )

    preview = text[:80].replace("\n", " ")
    return ok_response(f"Discord notified: {preview!r}")


def handle_health(config: Dict[str, Any]) -> Dict[str, Any]:
    webhook_url = str(config.get("webhook_url") or "").strip()
    if not webhook_url:
        return error_response("No webhook_url configured", retry=False)
    if not webhook_url.startswith("https://discord.com/api/webhooks/"):
        return error_response(
            f"webhook_url does not look like a Discord webhook: {webhook_url[:40]}",
            retry=False,
        )
    return ok_response("discord_notify config ok (webhook configured)")


def main() -> None:
    """Main entry point — read request from stdin, execute command, write response to stdout."""
    try:
        request = json.load(sys.stdin)
    except json.JSONDecodeError as e:
        json.dump(error_response(f"Invalid JSON input: {e}", retry=False), sys.stdout)
        sys.stdout.write("\n")
        sys.stdout.flush()
        sys.exit(1)

    command = request.get("command", "")
    config = request.get("config") if isinstance(request.get("config"), dict) else {}
    event = request.get("event", {})
    context = request.get("context", {})
    payload = event.get("payload", {}) if isinstance(event, dict) else {}

    if command == "handle":
        response = handle_command(config, payload, context)
    elif command == "poll":
        # Scheduled polls: dispatcher doesn't pass event payload for non-handle commands.
        # Fall back to config-level poll_message so schedules can include a message.
        if not payload.get("message") and not payload.get("content") and not payload.get("title"):
            poll_msg = str(config.get("poll_message") or "").strip()
            if poll_msg:
                payload = {**payload, "message": poll_msg}
        response = handle_command(config, payload, context)
    elif command == "health":
        response = handle_health(config)
    else:
        response = error_response(
            f"Unknown command: '{command}'. Supported: handle, poll, health"
        )

    json.dump(response, sys.stdout, separators=(",", ":"))
    sys.stdout.write("\n")
    sys.stdout.flush()


if __name__ == "__main__":
    main()
