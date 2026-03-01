#!/usr/bin/env python3
"""YouTube playlist poller plugin for Ductile Gateway (protocol v2).

Fetches the YouTube playlist Atom feed and emits events for new videos.
Uses plugin state to avoid duplicate processing and supports conditional
requests (ETag/Last-Modified) to reduce rate limiting.
"""
from __future__ import annotations

import json
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional, Tuple

ATOM_NS = {
    "atom": "http://www.w3.org/2005/Atom",
    "yt": "http://www.youtube.com/xml/schemas/2015",
    "media": "http://search.yahoo.com/mrss/",
}

PLAYLIST_ID_RE = re.compile(r"^[A-Za-z0-9_-]+$")
SLUG_RE = re.compile(r"[^a-z0-9]+")

DEFAULT_PROMPT = """You are writing a markdown file for the Astro summaries collection from the transcript.

Return ONLY markdown with YAML frontmatter matching this schema exactly:

---
created: '{created_at}'
id: {video_id}
metadata: {}
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
    events: Optional[List[Dict[str, Any]]] = None,
    state_updates: Optional[Dict[str, Any]] = None,
    logs: Optional[List[Dict[str, str]]] = None,
) -> Dict[str, Any]:
    out: Dict[str, Any] = {"status": "ok", "logs": logs or []}
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


def fetch_feed(
    url: str,
    timeout: int,
    user_agent: str,
    etag: str,
    last_modified: str,
) -> Tuple[Optional[str], str, str, bool]:
    headers = {"User-Agent": user_agent}
    if etag:
        headers["If-None-Match"] = etag
    if last_modified:
        headers["If-Modified-Since"] = last_modified

    req = urllib.request.Request(url, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read().decode("utf-8", errors="replace")
            return (
                body,
                resp.headers.get("ETag", ""),
                resp.headers.get("Last-Modified", ""),
                False,
            )
    except urllib.error.HTTPError as e:
        if e.code == 304:
            return None, etag, last_modified, True
        raise


def parse_entries(feed_xml: str) -> List[Dict[str, str]]:
    root = ET.fromstring(feed_xml)
    entries: List[Dict[str, str]] = []
    for entry in root.findall("atom:entry", ATOM_NS):
        video_id = entry.findtext("yt:videoId", default="", namespaces=ATOM_NS).strip()
        title = entry.findtext("atom:title", default="", namespaces=ATOM_NS).strip()
        published = entry.findtext("atom:published", default="", namespaces=ATOM_NS).strip()
        link_el = entry.find("atom:link", ATOM_NS)
        link = ""
        if link_el is not None:
            link = link_el.attrib.get("href", "")
        if not link and video_id:
            link = f"https://www.youtube.com/watch?v={video_id}"
        if not video_id:
            continue
        entries.append(
            {
                "video_id": video_id,
                "title": title,
                "published": published,
                "video_url": link,
            }
        )
    return entries


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


def handle_poll(config: Dict[str, Any], state: Dict[str, Any]) -> Dict[str, Any]:
    playlist_id = parse_playlist_id(str(config.get("playlist_id") or ""))
    playlist_url = str(config.get("playlist_url") or "").strip()
    if not playlist_id and playlist_url:
        playlist_id = parse_playlist_id(playlist_url)

    if not playlist_id:
        return error_response("Missing playlist_id or playlist_url in config", retry=False)

    feed_url = f"https://www.youtube.com/feeds/videos.xml?playlist_id={playlist_id}"
    timeout = int(config.get("request_timeout_seconds", 15))
    user_agent = str(
        config.get(
            "user_agent",
            "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
            "(KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
        )
    )

    etag = str(state.get("etag") or "")
    last_modified = str(state.get("last_modified") or "")

    try:
        feed_xml, new_etag, new_last_modified, not_modified = fetch_feed(
            feed_url,
            timeout=timeout,
            user_agent=user_agent,
            etag=etag,
            last_modified=last_modified,
        )
    except urllib.error.HTTPError as e:
        return error_response(f"Failed to fetch playlist feed: HTTP {e.code}", retry=e.code >= 500)
    except Exception as e:
        return error_response(f"Failed to fetch playlist feed: {e}", retry=True)

    if not_modified:
        return ok_response(
            logs=[{"level": "info", "message": "Playlist feed not modified"}],
            state_updates={"last_checked": iso_now()},
        )

    if not feed_xml:
        return ok_response(
            logs=[{"level": "warn", "message": "Playlist feed empty"}],
            state_updates={"last_checked": iso_now(), "etag": new_etag, "last_modified": new_last_modified},
        )

    entries = parse_entries(feed_xml)
    max_entries = int(config.get("max_entries", 50))
    entries = entries[:max_entries]

    seen_ids = state.get("seen_ids")
    seen_set = set(seen_ids or [])
    first_run = not seen_ids
    emit_existing = bool(config.get("emit_existing_on_first_run", True))

    new_entries = [entry for entry in entries if entry["video_id"] not in seen_set]

    if first_run and not emit_existing:
        updated_seen = list({entry["video_id"] for entry in entries} | seen_set)
        return ok_response(
            logs=[{"level": "info", "message": f"First run: recorded {len(updated_seen)} videos"}],
            state_updates={
                "seen_ids": updated_seen,
                "last_checked": iso_now(),
                "etag": new_etag,
                "last_modified": new_last_modified,
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
                published_date = datetime.fromisoformat(published_at.replace("Z", "+00:00")).date().isoformat()
            except ValueError:
                published_date = ""

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
            "playlist_url": playlist_url or feed_url,
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
            "playlist_url": playlist_url or feed_url,
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

    updated_seen = list({entry["video_id"] for entry in entries} | seen_set)

    return ok_response(
        events=events,
        state_updates={
            "seen_ids": updated_seen,
            "last_checked": iso_now(),
            "etag": new_etag,
            "last_modified": new_last_modified,
        },
        logs=[
            {
                "level": "info",
                "message": f"Emitted {len(events)} new playlist items (total seen {len(updated_seen)})",
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
    return ok_response(logs=[{"level": "info", "message": "youtube_playlist config ok"}])


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
