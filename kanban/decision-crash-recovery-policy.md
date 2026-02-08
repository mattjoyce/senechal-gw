---
id: 24
status: todo
priority: High
blocked_by: []
tags: [decision, mvp, ops, spec]
---

# Decide Crash Recovery Policy (MVP vs SPEC)

`MVP.md` and `SPEC.md` disagree on crash recovery behavior for orphaned `running` jobs after restart.

## Decision Needed
- **Option A (MVP.md):** On startup, find `running` jobs and mark them `dead` (no retry in MVP).
- **Option B (SPEC.md):** On startup, find `running` jobs, increment `attempt`, re-queue if under `max_attempts`, else `dead`.

## Recommendation
- Choose **Option B** even for MVP, since it exercises the at-least-once semantics and aligns with the main spec; it also de-risks later work (retry/backoff can still remain out of scope, but orphan recovery is a correctness requirement).

## Acceptance Criteria
- Decision recorded (which option, and why) in this card’s Narrative.
- Any resulting spec/MVP cleanup noted (e.g., update `MVP.md` if you want it aligned).

## Narrative

