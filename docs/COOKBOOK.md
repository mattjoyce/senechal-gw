---
audience: [1, 4]
form: tutorial
density: learner
verified: 2026-04-27
---

# Ductile Cookbook: Integration Patterns

Practical recipes for wiring Ductile **Connectors** and **Orchestrations** to solve real-world problems.

---

## Pattern: Automated Astro Staging Rebuild (Watch -> Trigger)

**Use case:** Rebuild your Astro staging site automatically whenever new AI-generated summaries are added to a specific folder.

### 1) Configure the `folder_watch` Connector

Set up a **Proactive Operation** (`poll`) to scan your content directory.

```yaml
# ~/.config/ductile/plugins.yaml
plugins:
  folder_watch:
    enabled: true
    schedules:
      - id: default
        every: 1m
    config:
      watches:
        - id: astro_summaries
          root: ${HOME}/site/src/content/summaries
          event_type: astro.summaries.changed
          recursive: true
          include_globs: ["**/*.md"]
          emit_mode: aggregate
```

### 2) Define the Rebuild Orchestration (Pipeline)

Create a **Pipeline** to respond to the `astro.summaries.changed` event.

```yaml
# ~/.config/ductile/pipelines.yaml
pipelines:
  - name: astro-rebuild-on-change
    on: astro.summaries.changed
    steps:
      - id: rebuild_staging
        uses: astro_rebuild_staging  # A sys_exec connector clone
```

### 3) Configure the Rebuild Connector

```yaml
# ~/.config/ductile/plugins.yaml
plugins:
  astro_rebuild_staging:
    enabled: true
    timeout: 15m
    config:
      command: "docker compose -f ${HOME}/admin/docker-compose.yml up -d --build"
      working_dir: "${HOME}/admin"
```

---

## Pattern: YouTube Playlist-to-Summary Pipeline

**Use case:** Automatically fetch, transcribe, and AI-summarise new videos from a YouTube playlist, then write the result to disk.

This is a multi-hop pipeline where each step passes its output (`result`) as the next step's `content` via baggage.

### 1) Configure the Playlist Watcher (Proactive)

```yaml
# ~/.config/ductile/plugins.yaml
plugins:
  youtube_playlist:
    enabled: true
    schedules:
      - every: 30m
        jitter: 2m
    timeout: 60s
    max_attempts: 2
    config:
      playlist_url: "https://www.youtube.com/playlist?list=PL5Rty1LvKaJ5GI4nqEzvEPTdgobgODlkk"
      output_dir: "/home/matt/tmp/ductile-output"
      filename_template: "{video_id}.md"
      max_entries: 50
      max_emit: 1                    # Only process one new video per run
      emit_existing_on_first_run: false
      transcript_language: en
```

### 2) Define the Processing Pipeline

```yaml
# ~/.config/ductile/pipelines.yaml
pipelines:
  - name: playlist-wisdom
    on: youtube.playlist_item
    steps:
      - id: transcript
        uses: youtube_transcript   # fetches transcript, result = raw text
      - id: summarize
        uses: fabric               # summarises transcript, result = markdown summary
      - id: write
        uses: file_handler         # writes summary to disk
```

### 3) Configure Supporting Plugins

```yaml
# ~/.config/ductile/plugins.yaml
plugins:
  youtube_transcript:
    enabled: true
    timeout: 60s
    max_attempts: 2
    config: {}

  fabric:
    enabled: true
    timeout: 120s
    max_attempts: 2
    config:
      FABRIC_DEFAULT_PATTERN: "summarize"

  file_handler:
    enabled: true
    timeout: 30s
    max_attempts: 1
    config:
      allowed_write_paths: "/home/matt/tmp/ductile-output"
      default_output_dir: "/home/matt/tmp/ductile-output"
```

### How it works

1. `youtube_playlist` polls the playlist every 30 min (with up to 2 min jitter).
2. For each new video, it emits a `youtube.playlist_item` event with `video_id` in the payload.
3. `youtube_transcript` fetches the transcript; its output (`result`) flows into the next step.
4. `fabric` summarises the transcript using the `summarize` pattern; its `result` becomes the markdown summary.
5. `file_handler` writes the summary to `{video_id}.md` in the output directory.

### Notes

- Set `max_emit: 1` to process one new video per poll cycle — avoids burst load on initial run.
- `emit_existing_on_first_run: false` means already-seen videos are skipped on restart.
- `youtube_playlist` uses `yt-dlp --flat-playlist` internally; ensure yt-dlp is installed and on PATH.
- For systemd services, add `~/.local/bin` to the service `Environment="PATH=..."` line.

---

## Pattern: Discord Notifications via Incoming Webhook

**Use case:** Send messages to a Discord channel from any pipeline step or scheduled trigger.

The `discord_notify` plugin wraps Discord's incoming webhook API. It exposes two schedulable-friendly commands:
- `handle` — called from pipeline steps (event-driven)
- `poll` — identical behaviour, but allowed in `schedules:` blocks (ductile forbids `handle` in schedules)

### 1) Configure the Plugin

```yaml
# ~/.config/ductile/plugins.yaml
plugins:
  discord_notify:
    enabled: true
    timeout: 15s
    max_attempts: 2
    config:
      webhook_url: "${DISCORD_WEBHOOK_URL}"   # or hard-code in tokens.yaml
      default_username: "Ductile"
```

### 2) Use in a Pipeline Step

The plugin reads `message`, `content`, `result`, or `title` from the payload (in that order).

```yaml
pipelines:
  - name: notify-on-build
    on: build.complete
    steps:
      - id: notify
        uses: discord_notify
        # payload.result from the previous step becomes the Discord message
```

Or pass a static message:

```yaml
pipelines:
  - name: notify-on-error
    on: job.failed
    steps:
      - id: alert
        uses: discord_notify
        payload:
          message: "A job failed — check the dashboard."
```

### 3) Use as a Scheduled Heartbeat

`poll` is the schedulable alias for `handle`. Use it with `schedules:` blocks to send timed notifications.

```yaml
plugins:
  discord_notify:
    schedules:
      # Daily 09:00 status ping
      - id: morning-ping
        cron: "0 9 * * *"
        command: poll
        payload:
          message: "Good morning — Ductile is running."
        not_on: [saturday, sunday]

      # Startup one-shot
      - id: boot-notify
        after: 30s
        command: poll
        payload:
          message: "Ductile started."
```

### Notes

- Discord hard-limits messages to 2000 characters; the plugin truncates automatically.
- 4xx errors (bad webhook, forbidden) are not retried. 5xx and network errors retry per `max_attempts`.
- `poll` and `handle` are identical in behaviour; the distinction is purely for ductile's scheduler validation.
- Store the webhook URL in `tokens.yaml` and reference it via `${VAR}` interpolation to keep it out of operational config files.

---

## Pattern: End-to-End: Playlist → Summary → Discord Notification

**Use case:** Combine the two patterns above — automatically process new playlist videos and notify a Discord channel when a summary is written.

```yaml
# ~/.config/ductile/pipelines.yaml
pipelines:
  - name: playlist-wisdom
    on: youtube.playlist_item
    steps:
      - id: transcript
        uses: youtube_transcript
      - id: summarize
        uses: fabric
      - id: write
        uses: file_handler
      - id: notify
        uses: discord_notify
        # After file_handler, result contains the output path or confirmation.
        # Or set a static title:
        payload:
          title: "New summary ready"
          # message will fall through to context.result from the write step
```

This gives you an automated, end-to-end pipeline with Discord confirmation for every new video processed.

---

## Pattern: Route YouTube vs Web URLs

**Use case:** Use the `if` classifier to emit different event types based on URL content.

### 1) Configure the classifier instance

```yaml
# ~/.config/ductile/plugins.yaml
plugins:
  check_youtube:
    enabled: true
    timeout: 30s
    max_attempts: 1
    config:
      field: text
      checks:
        - contains: "youtu.be"
          emit: youtube.url.detected
        - contains: "youtube.com"
          emit: youtube.url.detected
        - startswith: "http"
          emit: web.url.detected
        - default: text.received
```

### 2) Use it in a pipeline

```yaml
# ~/.config/ductile/pipelines.yaml
pipelines:
  - name: ai-dispatch
    on: discord.ai.command
    steps:
      - id: classify
        uses: check_youtube

  - name: youtube-wisdom
    on: youtube.url.detected
    steps:
      - uses: youtube_transcript
      - uses: fabric
      - uses: file_handler

  - name: web-summarize
    on: web.url.detected
    steps:
      - uses: fabric
```

### Notes

- `default` is a final fallback; omit it if you want no-match to error.
- The plugin passes the payload through unchanged.

---

## Adding your own patterns

Each recipe follows the same structure: configure a plugin, define a pipeline, wire the events. If you have a working integration worth sharing, add it here.
