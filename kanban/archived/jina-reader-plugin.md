---
id: 50
status: done
priority: Normal
blocked_by: []
assignee: "@claude"
tags: [plugin, webscraping, jina]
---

# Jina Reader Web Scraper Plugin

Build a plugin that uses Jina AI's Reader API (`r.jina.ai`) to convert web pages into clean markdown text.

## Job Story

When I want to extract clean content from a URL, I want a plugin that scrapes it via Jina Reader and returns markdown, so I can feed web content into downstream plugins for processing.

## Acceptance Criteria

- Plugin `jina-reader` with protocol v1
- `poll` command: scrape a configured URL, store content hash in state, emit `content_changed` event if content differs from last poll
- `handle` command: receive `{url}` in event payload, scrape it, emit `content_ready` event with markdown content
- Uses `https://r.jina.ai/{url}` endpoint (no auth required for basic use)
- Respects deadline_at for timeout
- Truncate large responses to configurable max size (default 100KB)
- Config keys: `url` (optional, for poll mode), `max_size` (optional)

## Implementation Notes

- Research Jina Reader API capabilities and response format
- Can be Python or TypeScript/Bun
- Hash comparison for change detection (sha256 of content)

## Narrative
- 2026-02-11: Implemented as Python plugin using stdlib only. Poll mode with SHA-256 change detection, handle mode for on-demand scraping, optional API key support. Tested all commands and error paths against live Jina Reader API. (by @claude)
