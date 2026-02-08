---
id: 22
status: backlog
priority: Normal
blocked_by: []
tags: [sprint-3, epic, webhooks]
---

# Sprint 3: Webhooks + /healthz

Add the HTTP listener for inbound webhooks and a local `/healthz` endpoint for ops visibility.

## Acceptance Criteria
- `webhooks.listen` binds and serves endpoints configured in `config.yaml`.
- HMAC-SHA256 signature verification is mandatory and returns `403` on failure with no details.
- Body size limits enforced (default 1 MB).
- `/healthz` returns status, uptime, queue depth, plugins loaded, circuits open (per SPEC).

## Narrative

