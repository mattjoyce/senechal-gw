#!/usr/bin/env python3
"""fetch: Retrieve raw webpage content via HTTP GET.

Protocol v2 plugin. Fetches a URL directly (no external API) and returns
the content as HTML, stripped plain text, or markdown.

Config keys:
  user_agent       - Custom UA string (default: ductile-fetch/1.0)
  timeout_seconds  - Request timeout in seconds (default: 30)
  follow_redirects - Follow HTTP redirects, true|false (default: true)
  output_format    - html | text | markdown (default: html)
                     markdown: sends Accept: text/markdown — sites that support
                     content negotiation (e.g. Cloudflare Markdown for Agents)
                     return pre-converted markdown; others fall back to HTML.
"""

import html.parser
import json
import sys
import urllib.error
import urllib.request

# ---------------------------------------------------------------------------
# Read request
# ---------------------------------------------------------------------------

request = json.loads(sys.stdin.read())
command = request.get("command", "handle")
config = request.get("config", {})
event = request.get("event", {})

USER_AGENT = config.get("user_agent", "ductile-fetch/1.0")
TIMEOUT = int(config.get("timeout_seconds", 30))
FOLLOW_REDIRECTS = str(config.get("follow_redirects", "true")).lower() != "false"
OUTPUT_FORMAT = config.get("output_format", "html").lower()


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def respond(status, result=None, error=None, retry=True, events=None, logs=None):
    resp = {"status": status}
    if error:
        resp["error"] = error
        resp["retry"] = retry
    if result is not None:
        resp["result"] = result
    if events:
        resp["events"] = events
    resp["logs"] = logs or []
    json.dump(resp, sys.stdout)
    sys.exit(0)


class _TextExtractor(html.parser.HTMLParser):
    """Minimal tag-stripping HTML parser."""

    _SKIP = {"script", "style", "head", "noscript"}

    def __init__(self):
        super().__init__()
        self._skip_depth = 0
        self._parts = []

    def handle_starttag(self, tag, attrs):
        if tag in self._SKIP:
            self._skip_depth += 1

    def handle_endtag(self, tag):
        if tag in self._SKIP and self._skip_depth:
            self._skip_depth -= 1

    def handle_data(self, data):
        if not self._skip_depth:
            stripped = data.strip()
            if stripped:
                self._parts.append(stripped)

    def text(self):
        return "\n".join(self._parts)


def html_to_text(raw_html):
    parser = _TextExtractor()
    parser.feed(raw_html)
    return parser.text()


def fetch_url(url):
    """Return (body_str, status_code, final_url, markdown_tokens)."""
    headers = {"User-Agent": USER_AGENT}
    if OUTPUT_FORMAT == "markdown":
        headers["Accept"] = "text/markdown, text/html"

    req = urllib.request.Request(url, headers=headers)

    opener = urllib.request.build_opener()
    if not FOLLOW_REDIRECTS:
        opener = urllib.request.build_opener(urllib.request.HTTPErrorProcessor())

    with opener.open(req, timeout=TIMEOUT) as resp:
        raw = resp.read()
        status_code = resp.status
        final_url = resp.url
        content_type = resp.headers.get("Content-Type", "")
        markdown_tokens = resp.headers.get("x-markdown-tokens")

    charset = "utf-8"
    ct = resp.headers.get_content_charset()
    if ct:
        charset = ct

    body = raw.decode(charset, errors="replace")

    # If we requested markdown but the server returned HTML, note the fallback
    server_sent_markdown = "text/markdown" in content_type

    return body, status_code, final_url, markdown_tokens, server_sent_markdown


# ---------------------------------------------------------------------------
# Command handlers
# ---------------------------------------------------------------------------

if command == "health":
    respond("ok", result="healthy", logs=[{"level": "info", "message": "healthy"}])

elif command == "handle":
    url = event.get("payload", {}).get("url") or event.get("url")
    if not url:
        respond(
            "error",
            error="event must include url",
            retry=False,
            logs=[{"level": "error", "message": "handle: no url in event payload"}],
        )

    try:
        body, status_code, final_url, markdown_tokens, server_sent_markdown = fetch_url(url)
    except urllib.error.HTTPError as exc:
        respond(
            "error",
            error=f"HTTP {exc.code}: {exc.reason}",
            events=[{"type": "fetch.failed", "payload": {"url": url, "error": f"HTTP {exc.code}: {exc.reason}"}}],
            logs=[{"level": "error", "message": f"fetch failed for {url}: HTTP {exc.code}"}],
        )
    except (urllib.error.URLError, OSError, ValueError) as exc:
        respond(
            "error",
            error=str(exc),
            events=[{"type": "fetch.failed", "payload": {"url": url, "error": str(exc)}}],
            logs=[{"level": "error", "message": f"fetch failed for {url}: {exc}"}],
        )

    effective_format = OUTPUT_FORMAT
    if OUTPUT_FORMAT == "markdown" and not server_sent_markdown:
        # Server didn't honour Accept: text/markdown — fall back to HTML
        content = body
        effective_format = "html"
    elif OUTPUT_FORMAT == "text":
        content = html_to_text(body)
    else:
        content = body

    logs = [
        {
            "level": "info",
            "message": f"fetched {url} → {status_code} ({len(content)} chars, format={effective_format})",
        }
    ]
    if final_url != url:
        logs.append({"level": "info", "message": f"redirected to {final_url}"})
    if OUTPUT_FORMAT == "markdown" and not server_sent_markdown:
        logs.append({"level": "warn", "message": "server did not return text/markdown; fell back to html"})

    payload = {
        "url": url,
        "final_url": final_url,
        "status_code": status_code,
        "content_length": len(content),
        "output_format": effective_format,
        "content": content,
    }
    if markdown_tokens is not None:
        payload["markdown_tokens"] = int(markdown_tokens)

    respond(
        "ok",
        result=f"fetched {url} ({len(content)} chars, format={effective_format})",
        events=[{"type": "fetch.completed", "payload": payload}],
        logs=logs,
    )

else:
    respond(
        "error",
        error=f"unknown command: {command}",
        retry=False,
        logs=[{"level": "error", "message": f"unknown command: {command}"}],
    )
