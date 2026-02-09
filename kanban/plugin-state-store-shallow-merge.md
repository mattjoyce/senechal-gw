---
id: 17
status: done
priority: High
blocked_by: []
tags: [sprint-1, mvp, state]
---

# Plugin State Store (SQLite) + Shallow Merge

Persist and update per-plugin state blobs in SQLite so plugins can be idempotent and manage tokens across runs.

## Acceptance Criteria
- Read full state blob for a plugin (default `{}` if missing).
- Apply `state_updates` as a shallow merge (top-level keys replaced).
- Enforce state size limit (SPEC: 1 MB) or document the MVP deviation on the card.

## Narrative
- 2026-02-08: Implemented plugin state store in `internal/state/store.go` with `Get()` (returns empty object for missing plugins), `ShallowMerge()` (replaces top-level keys from updates), and 1MB size limit enforcement per SPEC. Uses JSON marshaling for SQLite storage in `plugin_state` table with `UPSERT` semantics. Comprehensive tests verify empty defaults, shallow merge behavior, and size limit rejection. Merged via PR #1. (by @codex)
