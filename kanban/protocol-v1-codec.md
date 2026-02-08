---
id: 13
status: todo
priority: High
blocked_by: [9]
tags: [sprint-1, mvp, protocol]
---

# Protocol v1: Request/Response Codec

Define protocol v1 envelope types and implement strict JSON encode/decode so the core can communicate with plugins reliably.

## Acceptance Criteria
- Request envelope matches SPEC (protocol=1, job_id, command, config, state, event, deadline_at).
- Response envelope matches SPEC (status, error, retry, events, state_updates, logs).
- Non-JSON on stdout is treated as protocol error (job fails; capture stdout+stderr for debugging).

## Narrative

