# jina-reader

Scrape web pages via the Jina Reader API and return clean markdown.

## Commands
- `poll` (write): Fetch a configured URL and emit `content_changed` when the content hash changes.
- `handle` (write): Scrape a URL from the event payload.
- `health` (read): Return health status.

## Configuration
- `url`: URL to scrape in `poll` mode.
- `max_size`: Maximum content bytes to keep (default: 102400).
- `jina_api_key`: Optional API key for higher rate limits.

## Events
- `content_changed` (poll) with payload `url`, `content`, `content_hash`, `truncated`.
- `content_ready` (handle) with payload `url`, `content`, `content_hash`, `truncated`.

## Example
```yaml
plugins:
  jina-reader:
    enabled: true
    schedules:
      - every: 10m
    config:
      url: "https://example.com"
      max_size: 204800
```
