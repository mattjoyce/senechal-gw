# Cookbook

Practical patterns for wiring Ductile plugins together.

## Pattern: Rebuild Astro Staging When Summaries Change

**Use case:** Trigger a site rebuild whenever a new AI-generated summary markdown file appears.

### 1) Watch the summaries folder

Configure `folder_watch` to scan the summaries directory on a schedule and emit an event when files change.

```yaml
# ~/.config/ductile/plugins.yaml
plugins:
  folder_watch:
    enabled: true
    schedule:
      every: 1m
    timeout: 30s
    max_attempts: 1
    config:
      watches:
        - id: astro_summaries
          root: /mnt/Projects/matt_joyce/site/src/content/summaries
          event_type: astro.summaries.changed
          recursive: true
          include_globs: ["**/*.md"]
          emit_mode: aggregate
          emit_initial: false
          min_stable_age: 1s
```

### 2) Pipeline: route the event to a rebuild step

Use the pipeline DSL to trigger `astro_rebuild_staging` when the folder watcher emits the event.

```yaml
# ~/.config/ductile/pipelines.yaml
pipelines:
  - name: astro-rebuild-staging-on-summary-change
    on: astro.summaries.changed
    steps:
      - id: rebuild_staging
        uses: astro_rebuild_staging
```

### 3) Ensure includes

If you use include-mode config, add `pipelines.yaml`:

```yaml
# ~/.config/ductile/config.yaml
include:
  - api.yaml
  - plugins.yaml
  - pipelines.yaml
  - webhooks.yaml
```

### 4) Rebuild plugin configuration

The rebuild plugin can be a `sys_exec` clone with a fixed command:

```yaml
# ~/.config/ductile/plugins.yaml
plugins:
  astro_rebuild_staging:
    enabled: true
    timeout: 15m
    max_attempts: 1
    config:
      command: "docker compose -f /home/matt/admin/docker-compose.yml up -d --build"
      working_dir: "/home/matt/admin"
      timeout_seconds: 900
```

### 5) (Optional) Trigger via webhook

If you also want a manual rebuild trigger, you can wire `astro_rebuild_staging` to a webhook endpoint and call it directly.

```yaml
# ~/.config/ductile/webhooks.yaml
webhooks:
  endpoints:
    - name: astro_rebuild_staging
      path: /webhook/astro-rebuild-staging
      plugin: astro_rebuild_staging
      secret: "<shared secret>"
      signature_header: X-Ductile-Signature-256
```

```bash
payload='{"payload":{"reason":"manual rebuild"}}'
sig=$(printf '%s' "$payload" | \
  openssl dgst -sha256 -hmac '<secret>' -hex | awk '{print $2}')

curl -sS -X POST http://127.0.0.1:8091/webhook/astro-rebuild-staging \
  -H "Content-Type: application/json" \
  -H "X-Ductile-Signature-256: sha256=$sig" \
  -d "$payload"
```

### Notes

- `emit_mode: aggregate` emits a single event with lists of created/modified/deleted files.
- `min_stable_age` helps avoid partial writes being detected as changes.
- Use a longer rebuild timeout if the container build is slow.
