# file_watch

Scheduled single-file watcher that emits events on create/modify/delete transitions.

## Commands
- `poll` (write): Scan configured files and emit change events.
- `health` (read): Validate watch configuration and access.

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
