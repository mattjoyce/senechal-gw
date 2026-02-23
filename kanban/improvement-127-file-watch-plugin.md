---
id: 127
status: doing
priority: Normal
blocked_by: []
tags: [improvement, plugin, watcher, filesystem, scheduler, pipelines]
---

# Add `file_watch` Plugin (Scheduled Single-File Change Detector)

## Job Story

When I need automation to react to one high-signal file changing, I want a scheduled watcher plugin that emits a typed event only on change, so I can trigger pipelines without wasteful downstream polling.

## Problem

We need a reliable boundary trigger for file changes produced outside direct Ductile control. For single-file cases, using a directory-wide watcher is heavier than needed and makes semantics less explicit.

## Proposed Solution

Implement a new plugin at `plugins/file_watch/` with protocol v2 and scheduled `poll` support:

- `poll`: Evaluate configured file watches and emit change events.
- `health`: Validate configuration and file path accessibility.

Suggested plugin config shape:

```yaml
plugins:
  file_watch:
    enabled: true
    schedule:
      every: 30s
    config:
      watches:
        - id: prompt_md
          path: /data/inbox/prompt.md
          event_type: watch.file.prompt.changed
          strategy: sha256        # or mtime_size
          emit_initial: false
          emit_deleted: true
          min_stable_age: 2s
```

Runtime behavior per watch:

1. Read current file metadata/fingerprint.
2. Compare to previous state snapshot.
3. If created/modified/deleted, emit one event with payload details.
4. Update plugin state via `state_updates`.

Event envelope guidance:

- `type`: configured `event_type`
- `payload`: `watch_id`, `path`, `change_type`, `fingerprint`, `size`, `mtime`
- `dedupe_key`: `file_watch:<watch_id>:<fingerprint_or_deleted_marker>`

## Acceptance Criteria

- [x] `plugins/file_watch/manifest.yaml` defines `poll` and `health` commands.
- [x] `poll` handles create/modify/delete transitions for configured files.
- [x] No event is emitted when file fingerprint is unchanged.
- [x] `strategy: sha256` and `strategy: mtime_size` are both supported.
- [x] `emit_initial: false` suppresses first-run events for existing files.
- [x] `emit_deleted: true` emits a deletion event when tracked file disappears.
- [x] Emitted events include `dedupe_key` to prevent redundant downstream queueing.
- [x] Plugin state persists last-seen snapshot per `watch.id`.
- [x] Health check reports invalid/missing config clearly.
- [ ] Add at least one pipeline integration test: emitted event triggers downstream step via `pipelines[].on`.

## Non-Goals

- Real-time OS-level file notification (`inotify`/`fsnotify`) in MVP.
- Recursive directory scanning (covered by `folder_watch` card).

## Narrative
- 2026-02-23: Created card to introduce a focused single-file watcher plugin as an event source for pipeline triggering at external integration boundaries. (by @assistant)
- 2026-02-23: Implemented `plugins/file_watch` with protocol v2 `poll` and `health`, stateful fingerprint diffing (`sha256` and `mtime_size`), create/modify/delete detection, `emit_initial`/`emit_deleted`, `min_stable_age`, and event-level dedupe keys. Remaining item is a formal pipeline integration test case in Go. (by @assistant)
