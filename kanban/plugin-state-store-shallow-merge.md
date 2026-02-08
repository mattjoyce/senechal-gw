---
id: 17
status: todo
priority: High
blocked_by: [9, 11]
tags: [sprint-1, mvp, state]
---

# Plugin State Store (SQLite) + Shallow Merge

Persist and update per-plugin state blobs in SQLite so plugins can be idempotent and manage tokens across runs.

## Acceptance Criteria
- Read full state blob for a plugin (default `{}` if missing).
- Apply `state_updates` as a shallow merge (top-level keys replaced).
- Enforce state size limit (SPEC: 1 MB) or document the MVP deviation on the card.

## Narrative

