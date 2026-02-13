#!/usr/bin/env python3
"""YouTube transcript plugin for Senechal Gateway (protocol v1)."""
import html
import json
import os
import re
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional, Tuple


VIDEO_ID_RE = re.compile(r"^[A-Za-z0-9_-]{11}$")


def error_response(message: str, retry: bool = False) -> Dict[str, Any]:
    return {
        "status": "error",
        "error": message,
        "retry": retry,
        "logs": [{"level": "error", "message": message}],
    }


def is_transient_error(message: str) -> bool:
    lowered = (message or "").lower()
    markers = (
        "429",
        "too many requests",
        "timed out",
        "timeout",
        "temporarily unavailable",
        "connection reset",
        "connection refused",
    )
    return any(marker in lowered for marker in markers)


def parse_video_id(value: str) -> Optional[str]:
    value = (value or "").strip()
    if not value:
        return None

    if VIDEO_ID_RE.fullmatch(value):
        return value

    try:
        parsed = urllib.parse.urlparse(value)
    except Exception:
        return None

    host = (parsed.hostname or "").lower()
    path_parts = [p for p in parsed.path.split("/") if p]
    query = urllib.parse.parse_qs(parsed.query or "")

    if host in ("youtu.be", "www.youtu.be"):
        return path_parts[0] if path_parts and VIDEO_ID_RE.fullmatch(path_parts[0]) else None

    if host.endswith("youtube.com") or host.endswith("youtube-nocookie.com"):
        if path_parts and path_parts[0] == "watch":
            v = (query.get("v") or [""])[0]
            return v if VIDEO_ID_RE.fullmatch(v) else None
        if path_parts and path_parts[0] in ("shorts", "embed", "live"):
            v = path_parts[1] if len(path_parts) > 1 else ""
            return v if VIDEO_ID_RE.fullmatch(v) else None

    return None


def fetch_url(url: str, timeout: int, user_agent: str) -> str:
    req = urllib.request.Request(url, headers={"User-Agent": user_agent})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return resp.read().decode("utf-8", errors="replace")


def extract_json_object_from(text: str, marker: str) -> Optional[Dict[str, Any]]:
    idx = text.find(marker)
    if idx == -1:
        return None

    start = text.find("{", idx)
    if start == -1:
        return None

    in_string = False
    escape = False
    depth = 0
    end = -1
    for i in range(start, len(text)):
        c = text[i]
        if in_string:
            if escape:
                escape = False
            elif c == "\\":
                escape = True
            elif c == '"':
                in_string = False
            continue

        if c == '"':
            in_string = True
        elif c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                end = i + 1
                break

    if end == -1:
        return None

    raw = text[start:end]
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return None


def get_player_response(video_id: str, timeout: int, user_agent: str) -> Dict[str, Any]:
    watch_url = f"https://www.youtube.com/watch?v={video_id}"
    page = fetch_url(watch_url, timeout=timeout, user_agent=user_agent)

    for marker in ("ytInitialPlayerResponse = ", '"ytInitialPlayerResponse":'):
        parsed = extract_json_object_from(page, marker)
        if parsed:
            return parsed

    raise ValueError("Unable to parse YouTube player response from watch page")


def select_caption_track(caption_tracks: List[Dict[str, Any]], language: str) -> Optional[Dict[str, Any]]:
    lang = (language or "").strip().lower()
    if not caption_tracks:
        return None

    if not lang:
        return caption_tracks[0]

    for track in caption_tracks:
        code = (track.get("languageCode") or "").lower()
        if code == lang:
            return track

    for track in caption_tracks:
        code = (track.get("languageCode") or "").lower()
        if code.startswith(lang):
            return track

    return caption_tracks[0]


def parse_json3_transcript(raw_json: str) -> str:
    data = json.loads(raw_json)
    lines: List[str] = []
    for event in data.get("events", []):
        segs = event.get("segs", [])
        if not segs:
            continue
        text = "".join(seg.get("utf8", "") for seg in segs).strip()
        text = text.replace("\n", " ").strip()
        if text:
            lines.append(text)
    return "\n".join(lines).strip()


def parse_xml_transcript(raw_xml: str) -> str:
    root = ET.fromstring(raw_xml)
    lines: List[str] = []
    for node in root.findall(".//text"):
        content = "".join(node.itertext())
        content = html.unescape(content).replace("\n", " ").strip()
        if content:
            lines.append(content)
    return "\n".join(lines).strip()


def fetch_transcript(base_url: str, timeout: int, user_agent: str) -> Tuple[str, str]:
    # Prefer json3 because it's structured and keeps punctuation better.
    joiner = "&" if "?" in base_url else "?"
    json3_url = f"{base_url}{joiner}fmt=json3"

    try:
        raw = fetch_url(json3_url, timeout=timeout, user_agent=user_agent)
        transcript = parse_json3_transcript(raw)
        if transcript:
            return transcript, "json3"
    except Exception:
        pass

    raw_xml = fetch_url(base_url, timeout=timeout, user_agent=user_agent)
    transcript = parse_xml_transcript(raw_xml)
    if transcript:
        return transcript, "xml"
    raise ValueError("Transcript track found, but no transcript text could be parsed")


def parse_vtt_transcript(raw_vtt: str) -> str:
    lines: List[str] = []
    for raw_line in raw_vtt.splitlines():
        line = raw_line.strip()
        if not line:
            continue
        if line == "WEBVTT":
            continue
        if line.startswith("NOTE"):
            continue
        if "-->" in line:
            continue
        if line.isdigit():
            continue

        # Remove basic VTT markup tags.
        line = re.sub(r"<[^>]+>", "", line).strip()
        if line:
            lines.append(html.unescape(line))

    # De-duplicate adjacent repeated lines from auto-captions.
    compact: List[str] = []
    for line in lines:
        if not compact or compact[-1] != line:
            compact.append(line)
    return "\n".join(compact).strip()


def extract_lang_from_vtt_path(path: str) -> str:
    name = os.path.basename(path)
    m = re.search(r"\.([A-Za-z]{2,3}(?:-[A-Za-z]{2,4})?)\.vtt$", name)
    return m.group(1) if m else ""


def choose_vtt_file(paths: List[str], preferred_language: str) -> Optional[str]:
    if not paths:
        return None

    lang = (preferred_language or "").lower()
    if not lang:
        return paths[0]

    for path in paths:
        code = extract_lang_from_vtt_path(path).lower()
        if code == lang:
            return path
    for path in paths:
        code = extract_lang_from_vtt_path(path).lower()
        if code.startswith(lang):
            return path
    return paths[0]


def fetch_transcript_via_ytdlp(video_id: str, language: str, timeout: int) -> Tuple[str, str, str]:
    if not shutil_which("yt-dlp"):
        raise RuntimeError("yt-dlp not available")

    lang = (language or "").strip()
    lang_opts = "en,en.*,en-US,en-GB"
    if lang:
        short = lang.split("-")[0]
        lang_opts = f"{lang},{short}.*,{short}"

    with tempfile.TemporaryDirectory(prefix=f"senechal-yt-{video_id}-") as tmpdir:
        output_tpl = os.path.join(tmpdir, "%(id)s.%(ext)s")
        url = f"https://www.youtube.com/watch?v={video_id}"
        cmd = [
            "yt-dlp",
            "--skip-download",
            "--write-subs",
            "--write-auto-subs",
            "--sub-format",
            "vtt",
            "--sub-langs",
            lang_opts,
            "-o",
            output_tpl,
            url,
        ]

        proc = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=timeout,
        )
        if proc.returncode != 0:
            stderr = (proc.stderr or "").strip()
            raise RuntimeError(f"yt-dlp failed: {stderr or 'unknown error'}")

        vtt_files = [
            os.path.join(tmpdir, name)
            for name in os.listdir(tmpdir)
            if name.endswith(".vtt")
        ]
        if not vtt_files:
            raise RuntimeError("yt-dlp completed but no subtitle files were produced")

        chosen = choose_vtt_file(vtt_files, lang)
        if not chosen:
            raise RuntimeError("No usable VTT subtitle file found")

        with open(chosen, "r", encoding="utf-8", errors="replace") as f:
            transcript = parse_vtt_transcript(f.read())
        if not transcript:
            raise RuntimeError("Selected VTT subtitle file did not contain transcript text")

        detected_lang = extract_lang_from_vtt_path(chosen) or lang
        return transcript, detected_lang, "ytdlp_vtt"


def shutil_which(binary: str) -> Optional[str]:
    for directory in os.environ.get("PATH", "").split(os.pathsep):
        candidate = os.path.join(directory, binary)
        if os.path.isfile(candidate) and os.access(candidate, os.X_OK):
            return candidate
    return None


def handle_command(config: Dict[str, Any], state: Dict[str, Any], event: Dict[str, Any]) -> Dict[str, Any]:
    payload = event.get("payload", {})
    video_ref = (
        payload.get("url")
        or payload.get("video_url")
        or payload.get("video_id")
        or payload.get("id")
        or ""
    )
    language = payload.get("language") or payload.get("lang") or config.get("default_language") or "en"
    timeout = int(config.get("request_timeout_seconds", 20))
    user_agent = config.get(
        "user_agent",
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
    )

    video_id = parse_video_id(str(video_ref))
    if not video_id:
        return error_response(
            "Missing/invalid video reference. Provide payload.url or payload.video_id",
            retry=False,
        )

    title = ""
    track_language = str(language)
    is_auto_generated = False
    source_format = ""
    transcript = ""
    ytdlp_error = ""

    try:
        transcript, track_language, source_format = fetch_transcript_via_ytdlp(
            video_id=video_id,
            language=str(language),
            timeout=timeout,
        )
    except Exception as e:
        ytdlp_error = str(e)

    # Fallback scraper path if yt-dlp is unavailable/fails.
    if not transcript:
        try:
            player = get_player_response(video_id, timeout=timeout, user_agent=user_agent)
            video_details = player.get("videoDetails", {})
            title = video_details.get("title", "")
            captions = (
                player.get("captions", {})
                .get("playerCaptionsTracklistRenderer", {})
                .get("captionTracks", [])
            )
            if not captions:
                if ytdlp_error:
                    return error_response(
                        f"No transcript/captions available (yt-dlp error: {ytdlp_error})",
                        retry=is_transient_error(ytdlp_error),
                    )
                return error_response("No transcript/captions available for this video", retry=False)

            track = select_caption_track(captions, str(language))
            if not track or not track.get("baseUrl"):
                return error_response("Could not select a usable caption track", retry=False)

            transcript, source_format = fetch_transcript(track["baseUrl"], timeout=timeout, user_agent=user_agent)
            track_language = track.get("languageCode", "") or track_language
            is_auto_generated = bool(track.get("kind") == "asr")
        except urllib.error.URLError as e:
            if ytdlp_error:
                return error_response(
                    f"Failed transcript fetch (yt-dlp: {ytdlp_error}; scraper: {e})",
                    retry=True,
                )
            return error_response(f"Failed to fetch YouTube page/transcript: {e}", retry=True)
        except Exception as e:
            if ytdlp_error:
                combined = f"Failed transcript fetch (yt-dlp: {ytdlp_error}; scraper: {e})"
                return error_response(
                    combined,
                    retry=is_transient_error(combined),
                )
            return error_response(str(e), retry=False)

    if not title:
        # Best-effort title fetch for ytdlp path.
        try:
            player = get_player_response(video_id, timeout=timeout, user_agent=user_agent)
            title = player.get("videoDetails", {}).get("title", "")
        except Exception:
            title = ""

    out_payload = {
        "video_id": video_id,
        "video_url": f"https://www.youtube.com/watch?v={video_id}",
        "title": title,
        "language": track_language,
        "is_auto_generated": is_auto_generated,
        "transcript": transcript,
        "text": transcript,  # Alias for downstream text processors (e.g., fabric)
        "source_format": source_format,
    }

    return {
        "status": "ok",
        "events": [{"type": "youtube.transcript", "payload": out_payload}],
        "state_updates": {
            "last_run": datetime.now(timezone.utc).isoformat(),
            "last_video_id": video_id,
            "last_language": track_language,
            "executions_count": state.get("executions_count", 0) + 1,
        },
        "logs": [
            {
                "level": "info",
                "message": (
                    f"Fetched transcript for {video_id} "
                    f"({track_language or '?'}, {len(transcript)} chars, {source_format})"
                ),
            }
        ],
    }


def poll_command(state: Dict[str, Any]) -> Dict[str, Any]:
    return {
        "status": "ok",
        "state_updates": {"last_poll": datetime.now(timezone.utc).isoformat()},
        "logs": [
            {
                "level": "info",
                "message": "youtube_transcript poll command (no-op, event-driven)",
            }
        ],
    }


def health_command(state: Dict[str, Any]) -> Dict[str, Any]:
    return {
        "status": "ok",
        "state_updates": {"last_health_check": datetime.now(timezone.utc).isoformat()},
        "logs": [{"level": "info", "message": "youtube_transcript healthy"}],
    }


def main() -> None:
    request = json.load(sys.stdin)
    command = request.get("command", "")
    config = request.get("config", {})
    state = request.get("state", {})
    event = request.get("event", {})

    if command == "poll":
        response = poll_command(state)
    elif command == "handle":
        response = handle_command(config, state, event)
    elif command == "health":
        response = health_command(state)
    else:
        response = error_response(f"Unknown command: {command}")

    json.dump(response, sys.stdout)


if __name__ == "__main__":
    main()
