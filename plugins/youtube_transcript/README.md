# youtube_transcript

Fetch YouTube video transcripts by URL or video ID.

## Commands
- `poll` (write): No-op scheduled tick (event-driven plugin).
- `handle` (write): Fetch transcript for a video.
- `health` (read): Return health status.

## Configuration
- `default_language`: Default transcript language (e.g. `en`).
- `request_timeout_seconds`: Fetch timeout (seconds).
- `user_agent`: Custom user-agent for scraping fallback.
- `js_runtime_path`: Optional JS runtime path for yt-dlp (node/bun).

## Input (handle)
Payload fields: `url`, `video_url`, or `video_id`, plus optional `language`.

## Events
Emits `youtube.transcript` with payload including `video_id`, `video_url`, `title`,
`language`, `transcript`, `text`, and `source_format`.

## Example
```yaml
plugins:
  youtube_transcript:
    enabled: true
    config:
      default_language: en
      request_timeout_seconds: 20
```
