---
id: 131
status: in_progress
priority: Normal
blocked_by: []
tags: [plugin, routing, classifier, conditional]
---

# Add `if` Plugin (General-Purpose Field Classifier)

## Job Story

When a pipeline needs to branch based on payload content, I want a general-purpose classifier plugin so I can route events to different downstream pipelines without writing custom logic per use case.

## What It Does

Receives an event, inspects a configured field, walks an ordered list of checks, emits the first matching event type with the original payload passed through unchanged.

## Plugin Config (per instance in plugins.yaml)

```yaml
plugins:
  check_youtube:           # instance name — any name
    config:
      field: text          # payload field to inspect
      checks:
        - contains: "youtu.be"
          emit: youtube.url.detected
        - contains: "youtube.com"
          emit: youtube.url.detected
        - startswith: "http"
          emit: web.url.detected
        - default: text.received   # fallback — always matches
```

Each instance is a different use of the same plugin binary.

## Supported Condition Types (v1)

| Type | Description |
|------|-------------|
| `contains` | field contains substring (case-insensitive) |
| `startswith` | field starts with string (case-insensitive) |
| `endswith` | field ends with string (case-insensitive) |
| `equals` | exact match (case-insensitive) |
| `regex` | full Python regex match against field value |
| `default` | always matches — use as final fallback |

## Behaviour

- Checks evaluated in order; first match wins
- Original payload passed through to the emitted event unchanged
- If no check matches and no `default` is set → `status: error`
- Missing or empty field → treated as empty string (not an error)
- `field` supports dot notation for nested fields: `payload.source`

## Protocol

Standard protocol v2. Commands: `handle`, `health`.

- `handle` — evaluate and emit
- `health` — validate config (field specified, at least one check, all checks have valid type + emit)

## Example: URL Routing

```yaml
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

## Example: Status Check

```yaml
plugins:
  check_job_status:
    config:
      field: status
      checks:
        - equals: "error"
          emit: job.failed
        - default: job.ok
```

## Acceptance Criteria

- [ ] Plugin manifest at `plugins/if/manifest.yaml`
- [ ] `handle` evaluates checks in order, first match wins
- [ ] All six condition types work correctly
- [ ] Original payload passed through unchanged to emitted event
- [ ] `default` condition always matches if reached
- [ ] Missing field treated as empty string, not an error
- [ ] No matching check + no default → `status: error`
- [ ] `health` validates config and reports misconfigured checks
- [ ] Tests cover all condition types, default fallback, missing field, no-match error
- [ ] Works as `check_youtube` instance routing YouTube vs web URLs

## Narrative

- 2026-02-27: Created card. Needed for #129 discord-youtube pipeline to route `/ai <url>` to youtube-wisdom vs web-summarize based on URL content. General-purpose design allows reuse for any field-based routing decision. (by @assistant)
