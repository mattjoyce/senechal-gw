#!/usr/bin/env python3
"""YouTube playlist poller plugin for Ductile Gateway (protocol v2).

Fetches playlist entries via yt-dlp --flat-playlist and emits events for new
videos. Uses plugin state (seen_ids) to avoid duplicate processing.
"""
from __future__ import annotations

import json
import re
import shutil
import subprocess
import sys
import urllib.parse
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

PLAYLIST_ID_RE = re.compile(r"^[A-Za-z0-9_-]+$")
SLUG_RE = re.compile(r"[^a-z0-9]+")

DEFAULT_PROMPT = """You are writing a markdown file for the Astro summaries collection from the transcript.

Return ONLY markdown with YAML frontmatter matching this schema exactly:

---
created: '{created_at}'
id: {video_id}
metadata: {{}}
model_used: gpt-4o
output_format: markdown
prompt_used: youtube playlist wisdom
source_type: url
source_url: {video_url}
title: '{title_yaml}'
---

Then include:

# {title}

## Summary

## Key Wisdom
- bullet list

## Actionable Advice
- bullet list

Keep it concise and specific. Do not wrap the answer in code fences.
"""


def iso_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def error_response(message: str, retry: bool = False) -> Dict[str, Any]:
    return {
        "status": "error",
        "error": message,
        "retry": retry,
        "logs": [{"level": "error", "message": message}],
    }


def ok_response(
    *,
    result: str,
    events: Optional[List[Dict[str, Any]]] = None,
    state_updates: Optional[Dict[str, Any]] = None,
    logs: Optional[List[Dict[str, str]]] = None,
) -> Dict[str, Any]:
    out: Dict[str, Any] = {"status": "ok", "result": result, "logs": logs or []}
    if events:
        out["events"] = events
    if state_updates:
        out["state_updates"] = state_updates
    return out


def parse_playlist_id(value: str) -> Optional[str]:
    value = (value or "").strip()
    if not value:
        return None
    if PLAYLIST_ID_RE.fullmatch(value):
        return value
    try:
        parsed = urllib.parse.urlparse(value)
    except Exception:
        return None
    query = urllib.parse.parse_qs(parsed.query or "")
    playlist_id = (query.get("list") or [""])[0]
    if playlist_id and PLAYLIST_ID_RE.fullmatch(playlist_id):
        return playlist_id
    return None


def slugify(text: str) -> str:
    text = (text or "").strip().lower()
    text = SLUG_RE.sub("-", text)
    text = text.strip("-")
    return text or "video"


def safe_format(template: str, values: Dict[str, Any]) -> str:
    class SafeDict(dict):
        def __missing__(self, key: str) -> str:
            return ""

    return template.format_map(SafeDict(values))


def build_output_path(
    output_dir: str,
    filename_template: str,
    values: Dict[str, Any],
) -> str:
    filename = safe_format(filename_template, values)
    filename = filename.strip() or f"{values.get('video_id', 'video')}.md"
    if not filename.endswith(".md"):
        filename = f"{filename}.md"
    output_dir = (output_dir or "").strip()
    if output_dir:
        return f"{output_dir.rstrip('/')}/{filename}"
    return filename


def fetch_playlist_via_ytdlp(
    playlist_url: str,
    max_entries: int,
    timeout: int,
) -> List[Dict[str, str]]:
    """Fetch playlist entries using yt-dlp --flat-playlist.

    Returns a list of dicts with video_id, title, published (ISO 8601), video_url.
    """
    ytdlp = shutil.which("yt-dlp")
    if not ytdlp:
        raise RuntimeError("yt-dlp not found in PATH")

    cmd = [
        ytdlp,
        "--flat-playlist",
        "--dump-json",
        "--no-warnings",
        "--playlist-end", str(max_entries),
        playlist_url,
    ]

    result = subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        timeout=timeout,
    )

    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or "yt-dlp exited non-zero")

    entries: List[Dict[str, str]] = []
    for line in result.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            item = json.loads(line)
        except json.JSONDecodeError:
            continue
        video_id = str(item.get("id") or item.get("video_id") or "").strip()
        if not video_id:
            continue
        # yt-dlp upload_date is YYYYMMDD; convert to ISO 8601
        upload_date = str(item.get("upload_date") or "").strip()
        published = ""
        if len(upload_date) == 8:
            try:
                published = (
                    datetime.strptime(upload_date, "%Y%m%d")
                    .replace(tzinfo=timezone.utc)
                    .isoformat()
                )
            except ValueError:
                pass
        entries.append({
            "video_id": video_id,
            "title": str(item.get("title") or "").strip(),
            "published": published,
            "video_url": str(
                item.get("url") or f"https://www.youtube.com/watch?v={video_id}"
            ),
        })
    return entries


def handle_poll(config: Dict[str, Any], state: Dict[str, Any]) -> Dict[str, Any]:
    playlist_id = parse_playlist_id(str(config.get("playlist_id") or ""))
    playlist_url = str(config.get("playlist_url") or "").strip()
    if not playlist_id and playlist_url:
        playlist_id = parse_playlist_id(playlist_url)

    if not playlist_id:
        return error_response("Missing playlist_id or playlist_url in config", retry=False)

    effective_url = playlist_url or f"https://www.youtube.com/playlist?list={playlist_id}"
    max_entries = int(config.get("max_entries", 50))
    timeout = int(config.get("request_timeout_seconds", 60))

    try:
        entries = fetch_playlist_via_ytdlp(effective_url, max_entries=max_entries, timeout=timeout)
    except subprocess.TimeoutExpired:
        return error_response("yt-dlp timed out fetching playlist", retry=True)
    except Exception as e:
        return error_response(f"Failed to fetch playlist: {e}", retry=True)

    seen_ids = state.get("seen_ids")
    seen_set = set(seen_ids or [])
    first_run = not seen_ids
    emit_existing = bool(config.get("emit_existing_on_first_run", True))

    new_entries = [entry for entry in entries if entry["video_id"] not in seen_set]

    if first_run and not emit_existing:
        updated_seen = list({entry["video_id"] for entry in entries} | seen_set)
        result = f"First run: recorded {len(updated_seen)} videos, emitting none"
        return ok_response(
            result=result,
            logs=[{"level": "info", "message": result}],
            state_updates={
                "seen_ids": updated_seen,
                "last_checked": iso_now(),
            },
        )

    max_emit = int(config.get("max_emit", 5))
    if max_emit > 0:
        new_entries = list(reversed(new_entries))
        new_entries = list(reversed(new_entries[:max_emit]))

    output_dir = str(config.get("output_dir") or "").strip()
    filename_template = str(config.get("filename_template") or "{video_id}.md").strip()
    prompt_template = str(config.get("prompt_template") or DEFAULT_PROMPT)
    transcript_language = str(config.get("transcript_language") or "").strip()

    events: List[Dict[str, Any]] = []
    for entry in new_entries:
        slug = slugify(entry.get("title", ""))
        published_at = entry.get("published") or ""
        published_date = ""
        if published_at:
            try:
                published_date = datetime.fromisoformat(
                    published_at.replace("Z", "+00:00")
                ).date().isoformat()
            except ValueError:
                pass

        raw_title = entry.get("title", "")
        title_clean = " ".join(str(raw_title).split())
        title_yaml = title_clean.replace("'", "''")
        created_at = published_at or iso_now()

        values = {
            "title": title_clean,
            "title_yaml": title_yaml,
            "video_id": entry.get("video_id", ""),
            "video_url": entry.get("video_url", ""),
            "playlist_id": playlist_id,
            "playlist_url": effective_url,
            "published_at": published_at,
            "published_date": published_date,
            "created_at": created_at,
            "slug": slug,
        }

        payload: Dict[str, Any] = {
            "video_id": entry.get("video_id", ""),
            "video_url": entry.get("video_url", ""),
            "url": entry.get("video_url", ""),
            "title": title_clean,
            "published_at": published_at,
            "playlist_id": playlist_id,
            "playlist_url": effective_url,
            "output_path": build_output_path(output_dir, filename_template, values),
            "prompt": safe_format(prompt_template, values),
        }
        if transcript_language:
            payload["language"] = transcript_language

        events.append(
            {
                "type": "youtube.playlist_item",
                "payload": payload,
                "dedupe_key": f"youtube:playlist:{entry.get('video_id', '')}",
            }
        )

    emitted_ids = {e["payload"]["video_id"] for e in events}
    updated_seen = list(emitted_ids | seen_set)

    result = f"Emitted {len(events)} new playlist items (total seen {len(updated_seen)})"
    return ok_response(
        result=result,
        events=events,
        state_updates={
            "seen_ids": updated_seen,
            "last_checked": iso_now(),
        },
        logs=[
            {
                "level": "info",
                "message": result,
            }
        ],
    )


def handle_health(config: Dict[str, Any]) -> Dict[str, Any]:
    playlist_id = parse_playlist_id(str(config.get("playlist_id") or ""))
    playlist_url = str(config.get("playlist_url") or "").strip()
    if not playlist_id and playlist_url:
        playlist_id = parse_playlist_id(playlist_url)
    if not playlist_id:
        return error_response("Missing playlist_id or playlist_url in config", retry=False)
    ytdlp = shutil.which("yt-dlp")
    if not ytdlp:
        return error_response("yt-dlp not found in PATH", retry=False)
    result = f"youtube_playlist config ok (yt-dlp: {ytdlp})"
    return ok_response(result=result, logs=[{"level": "info", "message": result}])


def handle_request(request: Dict[str, Any]) -> Dict[str, Any]:
    command = request.get("command", "")
    config = request.get("config") if isinstance(request.get("config"), dict) else {}
    state = request.get("state") if isinstance(request.get("state"), dict) else {}

    if command == "poll":
        return handle_poll(config, state)
    if command == "health":
        return handle_health(config)

    return error_response(f"unknown command: {command}")


def main() -> None:
    request = json.load(sys.stdin)
    response = handle_request(request)
    json.dump(response, sys.stdout)


if __name__ == "__main__":
    main()
