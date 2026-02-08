---
id: 2
status: done
priority: High
blocked_by: []
tags: [architecture, events]
---

# Input/Event Model

Design the 6 input types: messages, heartbeats, crons, hooks, webhooks, agent-to-agent.

## Acceptance Criteria
- Each input type is defined with clear semantics
- Event schema/envelope format is specified
- Routing from input to handler is designed
- Priority and ordering rules are documented

## Narrative
- 2026-02-08: Created during initial project discussion. The primer identifies six input mechanisms that trigger agent behavior. Need to decide which are essential for v1 vs later. (by @assistant)
- 2026-02-08: Simplified to a unified model: all input types are just **producers** that submit jobs to a central work queue. Scheduler (heartbeat with fuzzy intervals), webhook receiver, CLI manual runs, and plugin output all feed the same queue. Serial dispatch. Config-declared routing for chaining. (by @assistant)
