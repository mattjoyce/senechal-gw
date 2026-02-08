---
id: 13
status: done
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
- **2026-02-08: Complete.** Implemented `internal/protocol` package with:
  - `types.go`: Request, Response, Event, LogEntry types matching SPEC §6
  - `codec.go`: EncodeRequest, DecodeResponse, DecodeResponseLenient functions
  - `codec_test.go`: Comprehensive table-driven tests (all passing)
  - Strict validation: status required, error message for error status, retry defaults to true
  - Lenient decoder captures raw stdout for debugging protocol errors

