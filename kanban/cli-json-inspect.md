---
id: 61
status: done
priority: Normal
blocked_by: [53, 58]
assignee: "@gemini"
tags: [compliance, cli, observability]
---

# Feature: Implement mandatory --json for `job inspect`

Per `docs/CLI_DESIGN_PRINCIPLES.md`, all "Read" verbs must support a `--json` flag for machine-readability by LLM operators. `job inspect` currently only supports human-readable output.

## Acceptance Criteria
- `senechal-gw job inspect <id> --json` returns structured JSON representing the execution lineage.
- JSON output includes baggage, artifacts, and job metadata for each hop.
- Unit tests verify JSON output format.

## Narrative
- 2026-02-12: Identified during CLI refactor review. (by @gemini)
