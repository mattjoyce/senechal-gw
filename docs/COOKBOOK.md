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
    schedule:
      every: 1m
    config:
      watches:
        - id: astro_summaries
          root: /home/matt/site/src/content/summaries
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
      command: "docker compose -f /home/matt/admin/docker-compose.yml up -d --build"
      working_dir: "/home/matt/admin"
```

---

## Pattern: Discord Notifications for YouTube Transcripts

**Use case:** When a YouTube transcript is fetched, send a summary directly to a Discord channel.

### 1) The YouTube Fetcher (Proactive)

```yaml
# ~/.config/ductile/plugins.yaml
plugins:
  youtube_transcript:
    enabled: true
    config:
      video_id: "dQw4w9WgXcQ"
      emit_event: youtube.transcript.ready
```

### 2) The Notification Orchestration

This pipeline uses **Baggage** to carry the `video_id` or a `channel_id` from the original trigger through to the final notification step.

```yaml
# ~/.config/ductile/pipelines.yaml
pipelines:
  - name: notify-discord-on-transcript
    on: youtube.transcript.ready
    steps:
      - id: summarize
        uses: fabric  # AI Analysis step
      - id: notify
        uses: sys_exec # Calls a discord webhook curl command
```

### 3) Using Baggage in the Notification Step

The `sys_exec` connector can access **Baggage** fields (from the `context` JSON) to customize the notification message.

```yaml
# In the sys_exec call (Operation)
config:
  command: "curl -X POST -H 'Content-Type: application/json' -d '{\"content\": \"Summary: $DUCTILE_PAYLOAD_RESULT\"}' $DISCORD_WEBHOOK_URL"
```

### Notes

- `emit_mode: aggregate` emits a single event with lists of created/modified/deleted files.
- `min_stable_age` helps avoid partial writes being detected as changes.
- Use a longer rebuild timeout if the container build is slow.

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

## Why these patterns matter

Ductile is a **"lightweight, open-source integration engine for the agentic era."** These patterns demonstrate its core functional goals:
- **Useful:** Solves real automation pain points.
- **Quick to Deploy:** YAML-based config, no heavy infra.
- **Extensible:** Mix and match polyglot connectors.

By grounding these recipes in the **"Integration Sphere,"** we provide the **"robust compound semantic grounding"** needed for humans to build reliable systems that can then be operated or optimized by LLMs.
