# youtube_playlist

Poll a YouTube playlist feed and emit events for new videos.

## Commands
- `poll` (write): Fetch playlist entries and emit new video events.
- `health` (read): Validate configuration and yt-dlp availability.

## Configuration
- `playlist_id` or `playlist_url`: Playlist identifier.
- `output_dir`: Base directory for downstream file writes.
- `filename_template`: Filename template (default: `{video_id}.md`).
- `prompt_template`: Prompt template for downstream summarizers.
- `max_entries`: Max playlist items to fetch (default: 50).
- `max_emit`: Max events to emit per poll.
- `emit_existing_on_first_run`: Emit existing items on first run (default: true).
- `transcript_language`: Preferred transcript language.
- `request_timeout_seconds`: yt-dlp timeout (seconds).

## Events
Emits `youtube.playlist_item` with payload including `video_id`, `video_url`, `title`,
`playlist_id`, `output_path`, and `prompt`.

## Example
```yaml
plugins:
  youtube_playlist:
    enabled: true
    schedules:
      - every: 30m
    config:
      playlist_url: "https://youtube.com/playlist?list=..."
      output_dir: /srv/summaries
      filename_template: "{video_id}.md"
      max_emit: 5
```
