---
id: 1
status: done
priority: High
blocked_by: []
tags: [architecture, core]
---

# Core Architecture Design

Define the runtime, event loop, and state management patterns for the Senechal Gateway.

## Acceptance Criteria
- Runtime model is defined (single process, event-driven)
- Event loop design is documented (queue-based, priority handling)
- State management approach is chosen (in-memory, persistent, hybrid)
- Interaction between runtime, loop, and state is clear

## Narrative
- 2026-02-08: Created during initial project discussion. Primer article identifies three foundational elements: Agent Runtime with Gateway, Event Loop, and State Management. (by @assistant)
- 2026-02-08: Decided on: Go compiled core, serial work queue as central abstraction, heartbeat scheduler with fuzzy intervals, SQLite state store, subprocess plugin protocol (JSON over stdin/stdout). All producers (scheduler, webhooks, CLI, plugin output) submit to a single work queue. Design doc written for critique. (by @assistant)
