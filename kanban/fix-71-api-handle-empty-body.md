---
id: 71
status: todo
priority: High
blocked_by: [68]
assignee: "@gemini"
tags: [bug, api, orchestration, fix]
---

# Fix: Ensure `handle` commands always receive an Event envelope

The API trigger for `handle` commands currently only wraps the payload if one is provided. If the request body is empty, no event envelope is created, causing plugins to fail.

## Acceptance Criteria
- `POST /trigger/{plugin}/handle` must always result in a `job.Payload` that is a serialized `protocol.Event`.
- If no body is provided, create an empty `api.trigger` event.
- Update `internal/api/handlers_test.go` to assert this wrapping behavior for both empty and non-empty bodies.

## Narrative
- 2026-02-12: Discovered during review of Card #68 fix. Empty body case was missed. (by @gemini)
