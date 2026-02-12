---
id: 70
status: done
priority: High
blocked_by: [69, 37]
assignee: "@gemini"
tags: [cli, config, llm-affordance, write, safety]
---

# Implement Config Mutation (Set) with Safety

Implement the "Write" affordances for the LLM operator to modify configuration safely using dry-runs and schema validation.

## Acceptance Criteria
- `config set <path>=<value>` modifies the underlying YAML files.
- **Mandatory `--dry-run`:** Any mutation must first be previewed as a diff.
- **Pre-execution Check:** Automatically runs `config check` logic before applying any `set`.
- Support for complex types (booleans, durations, lists) with validation.

## Practical Test Scenarios
1. **Scenario: Safe Disable**
   - Command: `senechal-gw config set plugins.echo.enabled=false --dry-run`
   - Expect: A diff showing the `enabled` field changing to `false`, with a "Dry run: no changes applied" message.
2. **Scenario: Invalid Type Rejected**
   - Command: `senechal-gw config set service.tick_interval=blue`
   - Expect: A non-zero exit code and an error message: "Invalid value 'blue' for type duration."
3. **Scenario: Full Mutation**
   - Command: `senechal-gw config set plugins.echo.schedule.every=10m`
   - Expect: The physical `plugins.yaml` file is updated and a success message is returned.

## Narrative
- 2026-02-12: Created to establish safe configuration pathways for LLMs. (by @gemini)
