---
id: 69
status: todo
priority: High
blocked_by: [58]
assignee: "@gemini"
tags: [cli, config, llm-affordance, read]
---

# Implement Config Inspection (Show/Get)

Implement the "Read" affordances for the LLM operator to surgically inspect configuration nodes and entities.

## Acceptance Criteria
- `config show [type:name]` returns the full or filtered configuration.
- `config get <path>` returns specific values using dot-notation (e.g. `plugins.echo.enabled`).
- Mandatory `--json` flag support for all output.
- Support for `type:name` addressing (e.g., `plugin:withings`, `pipeline:wisdom`).

## Practical Test Scenarios
1. **Scenario: Entity Discovery**
   - Command: `senechal-gw config show plugin:echo --json`
   - Expect: A JSON object containing only the `echo` plugin configuration.
2. **Scenario: Path Query**
   - Command: `senechal-gw config get plugins.echo.schedule.every`
   - Expect: The string `5m` (or JSON if requested).
3. **Scenario: Wildcard List**
   - Command: `senechal-gw config show plugin:* --json`
   - Expect: A JSON map of all discovered plugins and their configs.

## Narrative
- 2026-02-12: Created to fulfill RFC-004 Operator model. (by @gemini)
