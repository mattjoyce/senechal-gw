---
id: 128
status: doing
priority: High
blocked_by: []
tags: [improvement, plugin, watcher, filesystem, scheduler, pipelines, astro]
---

# Add `folder_watch` Plugin (Scheduled Directory Snapshot Watcher)

## Job Story

When an external process writes output files into a content directory, I want Ductile to detect those changes and trigger a pipeline, so follow-up automation (like site rebuild + RSS refresh) happens automatically.

## Use Case (Stress Test)

1. A Docker-triggered process runs a YouTube transcript summarization pipeline.
2. It writes a markdown file into an Astro content folder.
3. File appearance/change should trigger a Ductile pipeline.
4. That pipeline runs an Astro refresh command so RSS is updated.

## Problem

There is no native plugin to monitor a directory for file create/modify/delete changes and emit a routing event. Without this, operators must rely on ad hoc cron glue or tightly couple every writer pipeline to every downstream consumer.

## Proposed Solution

Implement `plugins/folder_watch/` with protocol v2:

- `poll`: Build directory snapshot, diff vs previous state, emit events on change.
- `health`: Validate root path and glob configuration.

Suggested config shape:

```yaml
plugins:
  folder_watch:
    enabled: true
    schedule:
      every: 15s
    config:
      watches:
        - id: astro_content
          root: /workspace/astro/src/content
          recursive: true
          include_globs: ["**/*.md"]
          exclude_globs: ["dist/**", ".astro/**", "node_modules/**"]
          ignore_hidden: true
          emit_mode: aggregate
          event_type: watch.folder.astro_content.changed
          min_stable_age: 2s
```

Event guidance (`emit_mode: aggregate`):

- `type`: configured `event_type`
- `payload`:
  - `watch_id`, `root`
  - `created`: `[]string`
  - `modified`: `[]string`
  - `deleted`: `[]string`
  - `snapshot_hash`
  - `changed_count`
- `dedupe_key`: `folder_watch:<watch_id>:<snapshot_hash>`

Pipeline example for Astro rebuild:

```yaml
pipelines:
  - name: refresh-astro-on-content-change
    on: watch.folder.astro_content.changed
    steps:
      - uses: astro_refresh
```

## Acceptance Criteria

- [x] `plugins/folder_watch/manifest.yaml` defines `poll` and `health` commands.
- [x] `poll` detects create/modify/delete changes under configured root.
- [x] Include/exclude glob filters are applied correctly.
- [x] `emit_mode: aggregate` emits one event per poll for a watch when any changes exist.
- [x] No event is emitted when snapshot hash is unchanged.
- [x] Emitted events include `dedupe_key` derived from watch + snapshot.
- [x] State stores enough snapshot data to detect deltas on next run.
- [x] State growth is bounded/documented to stay within plugin state limits.
- [x] File stability guard (`min_stable_age`) avoids triggering on partially written files.
- [ ] End-to-end validation proves: writing a markdown file triggers an Astro refresh pipeline once.

## Non-Goals

- Native filesystem event subscriptions (`fsnotify`) in MVP.
- Full content-diff payloads (path-level metadata is sufficient).

## Narrative
- 2026-02-23: Created card to support directory-driven pipeline triggers, with Astro RSS refresh as the primary stress-test scenario for external file-producing workloads. (by @assistant)
- 2026-02-23: Implemented `plugins/folder_watch` with protocol v2 `poll` and `health`, recursive/non-recursive scanning, include/exclude glob filtering, hidden-file suppression, aggregate/per-file emit modes, snapshot hashing, event dedupe keys, stability delay, and state/event caps (`max_files`, `max_events`) for bounded growth. Remaining item is explicit Astro end-to-end validation wiring. (by @assistant)
