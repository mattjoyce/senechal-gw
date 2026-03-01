# folder_watch

Scheduled directory watcher that emits aggregate or per-file change events for new/modified/deleted files.

## Commands
- `poll` (write): Scan configured directories and emit change events.
- `health` (read): Validate watch configuration and access.

## Configuration
`watches`: list of watch entries with:
- `id`, `root`, `event_type` (required)
- `recursive`: scan subdirectories (default: true)
- `include_globs`, `exclude_globs`
- `ignore_hidden`: ignore dotfiles (default: true)
- `emit_mode`: `aggregate` or `per_file`
- `emit_initial`: emit initial snapshot (default: false)
- `min_stable_age`: seconds or duration string
- `strategy`: `mtime_size` or `sha256`
- `max_files`, `max_events`

## Events
`aggregate` mode emits one event with lists of `created`, `modified`, `deleted`. `per_file` emits per-path events with `change_type` metadata.

## Example
```yaml
plugins:
  folder_watch:
    enabled: true
    schedules:
      - every: 1m
    config:
      watches:
        - id: summaries
          root: /srv/content/summaries
          event_type: summaries.changed
          include_globs: ["**/*.md"]
          emit_mode: aggregate
```
