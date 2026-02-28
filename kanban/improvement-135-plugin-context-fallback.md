---
id: 135
status: todo
priority: Normal
blocked_by: []
tags: [improvement, plugins, context, baggage]
---

# Plugin Context (Baggage) Fallback Standard

## Job Story

When plugins are chained in pipelines, I want a clear standard for when a plugin should read from `request.context` (baggage) so that downstream steps reliably access upstream values without requiring every plugin to re-emit the same payload keys.

## Problem

Payloads are per-event; the accumulated context ledger exists but plugins must opt-in to read it. Today only a subset of plugins (e.g., `fabric`, `file_handler`) fall back to context. This is inconsistent and makes pipelines fragile when intermediate plugins emit narrow payloads.

## Proposal

Define a standard for plugin behavior:

- Plugins should **prefer `event.payload`** for step-specific inputs.
- Plugins should **fall back to `request.context`** for missing values when appropriate.
- Document which fields are expected to be read from context.

## Questions

- Should *all* first-party plugins adopt payload→context fallback by default?
- Are there exceptions (security/PII, large fields, or action-only plugins)?
- Should the core merge selected context keys into payload automatically?

## Acceptance Criteria

- [ ] Document the standard in `docs/PLUGIN_DEVELOPMENT.md`.
- [ ] Decide which first-party plugins should implement fallback.
- [ ] Update those plugins or explicitly justify exceptions.

## Narrative

- 2026-02-27: Card created to formalize how plugins should use baggage/context. (by @assistant)
