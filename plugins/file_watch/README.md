# file_watch

Scheduled single-file watcher that emits events on create/modify/delete transitions.

## Commands
- `poll` (write): Scan configured files, emit change events, and produce the current watcher snapshot.
- `health` (read): Validate watch configuration and access without writing durable state.

## Configuration
`watches`: list of watch entries with:
- `id` (required)
- `path` (required)
- `event_type` (required)
- `strategy`: `sha256` or `mtime_size` (default: `sha256`)
- `emit_initial`: emit create event on first scan (default: false)
- `emit_deleted`: emit delete events (default: true)
- `min_stable_age`: seconds or duration string before considering a file stable

## Events
Emits `event_type` with payload containing `watch_id`, `path`, `change_type`, and fingerprint/size metadata.

## Persistence
Successful `poll` runs emit a snapshot state shaped as:
- `watches`
- `last_poll_at`

Core records that snapshot as append-only `plugin_facts` rows with fact type `file_watch.snapshot` and keeps `plugin_state` as the latest compatibility snapshot for existing readers.

## Example
```yaml
plugins:
  file_watch:
    enabled: true
    schedules:
      - every: 30s
    config:
      watches:
        - id: config_file
          path: /etc/myapp/config.yaml
          event_type: config.changed
          strategy: mtime_size
```
