---
id: 141
status: todo
priority: Normal
blocked_by: []
tags: [improvement, scheduler, spec]
---

# Scheduler Review (Intervals, Jitter, Windows)

## Problem

The scheduler has grown beyond the original MVP assumptions. We need a focused review to confirm that `every` intervals, jitter handling, and preferred windows match operator expectations and the DSL design.

## Goals

- Validate the supported `every` syntax (duration + aliases + day/week extensions).
- Confirm jitter behavior (per-run randomization vs per-tick).
- Confirm preferred window snapping behavior and edge cases.
- Reconcile config validation errors with scheduler runtime behavior.

## Acceptance Criteria

- [ ] Review current implementation + spec sections in `docs/ARCHITECTURE.md`.
- [ ] Document any discrepancies and decide whether to change code or spec.
- [ ] If changes are needed, update the scheduler tests and docs.
- [ ] Add a minimal operator-facing note to `docs/OPERATOR_GUIDE.md` if behavior changes.

## Notes

- Keep it small: focus on schedule parsing + run timing semantics.
- Consider a follow-on card if cron-style scheduling is desired.
