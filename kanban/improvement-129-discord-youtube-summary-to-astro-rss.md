---
id: 129
status: todo
priority: High
blocked_by: []
tags: [improvement, discord, youtube, pipelines, astro, rss, e2e]
---

# Discord -> YouTube Summary -> Astro Markdown + RSS Refresh

## Job Story

When I request a YouTube video summary from Discord, I want Ductile to generate the summary, save it as a markdown file in my Astro content folder, and trigger an Astro refresh so `mattjoyce.ai` publishes an updated RSS feed.

## Outcome Goal

Deliver a reliable end-to-end flow from Discord command to published site update:

1. Discord request received by Ductile.
2. YouTube summary workflow runs.
3. Markdown output is written to Astro content.
4. Site refresh pipeline runs.
5. RSS on `mattjoyce.ai` reflects the new summary.

## Problem

The building blocks exist (inbound triggers, pipeline execution, watchers), but the full production flow is not yet wired and validated as one dependable user-facing capability.

## Proposed Solution

Implement an integrated pipeline with explicit contracts between stages:

- Ingress: accept Discord-triggered request payload (video URL + optional title/tags).
- Summarization: invoke existing YouTube summarization skill/plugin chain.
- Persistence: write normalized markdown frontmatter + body into Astro content directory.
- Change detection: use `file_watch` or `folder_watch` to emit deterministic content-change events.
- Publish: route watch event to Astro refresh step (build/content refresh) and verify RSS update.
- Observability: log correlation IDs from Discord request through summary generation, file write, and publish event.

## Accepted Input (Current Scope)

Use the Discord command format below as the initial supported request shape:

`/ai "summaries the following, and out line practical learning and wisdom" https://youtu.be/XNzRIlcsOEQ?si=ibEyKoTz1uu8xSMP`

Implementation expectations for this scope:

- Treat the quoted text as the summary instruction prompt.
- Extract the YouTube URL from the trailing argument.
- Route this into a normalized internal event payload:
  - `instruction_prompt`
  - `video_url`
  - `request_source=discord`

## Proposed Implementation Notes

- Reuse existing cards/features where possible:
  - `improvement-125-inbound-webhook-get-endpoint.md` for inbound command handling.
  - `improvement-127-file-watch-plugin.md` and `improvement-128-folder-watch-plugin.md` for filesystem change events.
  - Existing router + scheduler + dispatch path for orchestration.
- Prefer one primary event contract:
  - `discord.youtube.summary.requested` (ingress)
  - `watch.folder.astro_content.changed` or `watch.file.summary.changed` (publish trigger)
- Add idempotency controls (`dedupe_key`) to prevent duplicate runs for repeated Discord retries.

## Acceptance Criteria

- [ ] A Discord `/ai "<instruction>" <youtube_url>` request triggers a Ductile pipeline run.
- [ ] The example command is accepted end-to-end:
  - `/ai "summaries the following, and out line practical learning and wisdom" https://youtu.be/XNzRIlcsOEQ?si=ibEyKoTz1uu8xSMP`
- [ ] The pipeline produces one markdown file in configured Astro content path with valid frontmatter.
- [ ] The markdown filename/path strategy is deterministic and collision-safe.
- [ ] A watch event is emitted after the markdown write and routed to Astro refresh pipeline.
- [ ] Astro refresh step completes successfully and logs are visible in Ductile job records.
- [ ] RSS feed includes the new summary entry after refresh.
- [ ] Duplicate Discord deliveries of the same request do not create duplicate posts.
- [ ] One runbook documents local/dev verification and production verification steps.

## Verification Plan (MVP)

1. Trigger test request (Discord-equivalent payload) with a known YouTube URL.
2. Confirm summary job output and inspect generated markdown file.
3. Confirm watch event emission and routed Astro refresh job.
4. Verify RSS includes new item and site serves updated content.
5. Re-send same request and confirm dedupe behavior.

## Risks / Open Questions

- Exact Discord ingress mechanism (slash command webhook vs bot relay) may affect payload/auth handling.
- Astro refresh mechanism must be clearly defined for deployment target (local build, CI trigger, or remote command).
- Watcher cap behavior (`max_files`, `max_events`) must be configured to avoid dropped/false events for content directories.

## Non-Goals

- Building a full Discord bot management UX.
- Real-time websocket/file subscription outside scheduled/watcher model.
- Reworking Astro theme/content model beyond required summary content contract.

## Narrative

- 2026-02-23: Created end-to-end story card for Discord-requested YouTube summary publication into Astro with RSS refresh on `mattjoyce.ai`, using existing Ductile components plus targeted integration work. (by @assistant)
- 2026-02-23: Updated scope to explicitly support Discord command format `/ai "<instruction>" <youtube_url>` and captured the accepted example prompt+URL contract for implementation/testing. (by @assistant)
