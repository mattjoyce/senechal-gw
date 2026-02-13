#!/usr/bin/env python3
"""jina-reader: Scrape web pages via Jina Reader API (r.jina.ai).

Protocol v1 plugin. Converts URLs to clean markdown via Jina's free
Reader API. Supports poll (configured URL) and handle (URL from event).

Config keys:
  url        - URL to scrape in poll mode (optional)
  max_size   - Max content bytes to keep (default: 102400 = 100KB)
  jina_api_key - Optional API key for higher rate limits
"""

import hashlib
import json
import sys
import urllib.request
import urllib.error

# --- Read request from stdin ---

request = json.loads(sys.stdin.read())
command = request.get("command", "poll")
config = request.get("config", {})
state = request.get("state", {})
event = request.get("event", {})

max_size = int(config.get("max_size", 102400))


def respond(status, error=None, events=None, state_updates=None, logs=None):
    """Write protocol v1 response to stdout and exit."""
    resp = {"status": status}
    if error:
        resp["error"] = error
        resp["retry"] = True
    if events:
        resp["events"] = events
    if state_updates:
        resp["state_updates"] = state_updates
    resp["logs"] = logs or []
    json.dump(resp, sys.stdout)
    sys.exit(0)


def fetch_via_jina(url):
    """Fetch URL content as markdown via Jina Reader API."""
    jina_url = f"https://r.jina.ai/{url}"
    headers = {
        "Accept": "text/plain",
        "User-Agent": "ductile-gw/jina-reader",
    }
    api_key = config.get("jina_api_key")
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"

    req = urllib.request.Request(jina_url, headers=headers)
    with urllib.request.urlopen(req, timeout=30) as resp:
        content = resp.read(max_size + 1)

    truncated = len(content) > max_size
    content = content[:max_size]
    return content.decode("utf-8", errors="replace"), truncated


def content_hash(text):
    """SHA-256 hash of content for change detection."""
    return hashlib.sha256(text.encode("utf-8")).hexdigest()[:16]


# --- Command handlers ---

if command == "health":
    respond("ok", logs=[{"level": "info", "message": "healthy"}])

elif command == "poll":
    url = config.get("url")
    if not url:
        respond("error", error="config.url required for poll command",
                logs=[{"level": "error", "message": "no url configured for poll"}])

    try:
        markdown, truncated = fetch_via_jina(url)
    except (urllib.error.URLError, urllib.error.HTTPError, OSError) as exc:
        respond("error", error=f"fetch failed: {exc}",
                logs=[{"level": "error", "message": f"fetch failed for {url}: {exc}"}])

    new_hash = content_hash(markdown)
    old_hash = state.get("content_hash", "")
    changed = new_hash != old_hash

    events = []
    if changed:
        events.append({
            "type": "content_changed",
            "payload": {
                "url": url,
                "content": markdown,
                "content_hash": new_hash,
                "truncated": truncated,
            },
        })

    logs = [{"level": "info", "message": f"polled {url} (hash={new_hash}, changed={changed})"}]
    if truncated:
        logs.append({"level": "warn", "message": f"content truncated to {max_size} bytes"})

    respond("ok",
            events=events,
            state_updates={"content_hash": new_hash, "last_url": url},
            logs=logs)

elif command == "handle":
    url = event.get("payload", {}).get("url") or event.get("url")
    if not url:
        respond("error", error="event must include url",
                logs=[{"level": "error", "message": "handle: no url in event payload"}])

    try:
        markdown, truncated = fetch_via_jina(url)
    except (urllib.error.URLError, urllib.error.HTTPError, OSError) as exc:
        respond("error", error=f"fetch failed: {exc}",
                logs=[{"level": "error", "message": f"fetch failed for {url}: {exc}"}])

    logs = [{"level": "info", "message": f"scraped {url} ({len(markdown)} bytes)"}]
    if truncated:
        logs.append({"level": "warn", "message": f"content truncated to {max_size} bytes"})

    respond("ok",
            events=[{
                "type": "content_ready",
                "payload": {
                    "url": url,
                    "content": markdown,
                    "content_hash": content_hash(markdown),
                    "truncated": truncated,
                },
            }],
            state_updates={"last_url": url},
            logs=logs)

else:
    respond("error", error=f"unknown command: {command}",
            logs=[{"level": "error", "message": f"unknown command: {command}"}])
