---
id: 20
status: done
priority: High
blocked_by: []
tags: [sprint-1, mvp, testing]
---

# Echo Plugin + E2E Runbook

Create a trivial `echo` plugin and a small runbook to validate the end-to-end loop works.

## Acceptance Criteria
- `plugins/echo/manifest.yaml` and runnable entrypoint exist.
- Plugin reads request JSON from stdin and writes a valid response JSON to stdout.
- `state_updates.last_run` (or similar) persists to SQLite across runs.
- Runbook covers: normal run, plugin failure path, and hung plugin timeout test (per MVP).

## Narrative

- 2026-02-08: Updated `plugins/echo` to conform to the protocol v1 manifest schema used by the core (`commands` as a list; `config_keys.required/optional`), and made `run.sh` executable so discovery/trust checks pass. Extended `run.sh` to support deterministic E2E test modes (`error`, `hang`, `protocol_error`) via `config.mode`, while always emitting protocol v1 response JSON with `state_updates.last_run` and `logs`. Added an end-to-end validation runbook at `docs/E2E_ECHO_RUNBOOK.md` covering happy/error/timeout/protocol/crash-recovery scenarios. Created comprehensive E2E tests in `internal/e2e/echo_plugin_test.go` validating full loop integration: discovery, protocol conformance, error modes, and protocol error handling. All tests passing. Merged via PR #5. (by @codex)
