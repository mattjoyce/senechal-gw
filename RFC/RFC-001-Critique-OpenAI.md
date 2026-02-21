# RFC-001 Critique: Ductile (Constructive Review)

**Status:** Historical Critique
Target: `RFC-001-ductile-gateway.md` (Draft, dated 2026-02-08)

## High-Level Take

The overall direction is strong: a small Go core with subprocess plugins is a pragmatic boundary that keeps the system understandable and lets connectors evolve independently. The “work queue as the one abstraction” is a good unifying model for scheduler, webhooks, manual runs, and downstream routing.

Most of what’s missing is *operational semantics*: exactly-once vs at-least-once, retries, timeouts, crash recovery details, and protocol/versioning. If you tighten those, you’ll avoid the class of bugs that make integration gateways feel “haunted” (duplicated actions, missing actions, jobs stuck forever, silent failures).

## What’s Working Well

- **Clear product target**: “lightweight personal integration gateway” is a great constraint; the serial dispatch choice matches it.
- **Good boundary choice**: subprocess + JSON is language-agnostic, isolates failures, and keeps plugin dev friction low.
- **Central queue**: forcing *all* triggers through one queue simplifies observability, persistence, and later features (retries, DLQ).
- **Config-declared routing**: keeps plugins dumb and makes the system reconfigurable without code changes.
- **SQLite for state**: correct for a zero-ops personal service.

## Major Gaps / Risks (Ordered By Severity)

### 1. Delivery Semantics Are Undefined (Duplicates and Loss)

Right now it’s unclear whether the system guarantees:
- at-most-once (might drop work),
- at-least-once (might duplicate work),
- exactly-once (hard, usually not worth it).

For personal automations, **at-least-once + idempotency** is usually the sweet spot. But that requires RFC-level guidance:
- Every job should have a stable `job_id` (UUID) and a `dedupe_key` option (e.g., `plugin:event_type:external_id`).
- Every emitted event should have `event_id` (UUID) and optionally a `dedupe_key`.
- Retries must be explicit: policy, max attempts, backoff, which errors retry, and how plugins signal retryability.

Without this, “notify” style plugins will double-send, and “poll” style plugins will re-import the same data.

### 2. Crash Recovery + “In-Flight” Job Handling Needs a Spec

You mention SQLite-backed persistence, but the RFC doesn’t define what happens to:
- a job marked active when the process crashes,
- a plugin that was running when the core restarts,
- partially processed downstream enqueues.

Minimum viable spec:
- job statuses: `queued | running | succeeded | failed | dead` (or similar).
- when a job transitions to `running` and how it’s checkpointed.
- on startup: any `running` jobs become `queued` again (with incremented attempt) after a “stale” timeout.
- record `attempt`, `last_error`, and `next_run_at` (for backoff scheduling).

### 3. Plugin Timeout / Cancellation / Resource Control

The RFC lists timeout handling as an open question, but it’s not optional in practice:
- A single hung plugin will halt the serial dispatcher indefinitely.
- Some plugins will occasionally hang on network calls or OAuth flows.

Recommend:
- per-command timeouts (`poll`, `handle`, etc.) in config with defaults (e.g., 30s/2m/5m).
- hard kill on timeout (`SIGKILL` after graceful window).
- max stdout/stderr size limits to avoid unbounded memory use.
- optional concurrency later, but even in serial mode you need *resource caps*.

### 4. Protocol Spec Is Too Loose (Will Drift)

“JSON lines over stdin/stdout” is fine, but you need a tighter envelope to prevent plugin/core mismatch and improve debuggability.

Add to the RFC:
- protocol `version` field in both request and response.
- include `job_id`, `plugin`, `command`, `deadline_at` in the request.
- response should include:
  - `status`: `ok | error`,
  - `retry`: boolean (or `retry_after_ms`),
  - `events`: list (with required `type`, `payload`, and `event_id`),
  - `state_updates`: structured object (and size limits),
  - `logs`: structured entries (or omit and rely on stderr).
- specify how errors are represented (error codes vs message).

Also: define *framing* precisely:
- newline-delimited JSON is OK, but what about large payloads? You’ll eventually hit practical limits.
  - Option A: keep payloads small, require plugins to store blobs somewhere (file path) and pass references.
  - Option B: support length-prefixed messages (a bit more work, more robust).

### 5. Webhook Security and Replay Protection

You have `secret` for webhooks, but the RFC doesn’t specify:
- signature algorithm (HMAC-SHA256? provider-specific?),
- timestamp tolerance and replay protection,
- request size limits,
- content-type requirements,
- logging redaction.

This matters even “for personal use” because webhook endpoints get scanned. Even if bound to `127.0.0.1`, many setups later put it behind a reverse proxy.

### 6. Config Example Has an Apparent Bug / Ambiguity

In `config.yaml`, there’s a duplicated `plugins:` key separated by `---`:

```yaml
plugins:
---
plugins:
  withings:
```

If that’s intentional (YAML document separator), the loader behavior must be defined. If not intentional, it will confuse readers and implementers and should be fixed in the RFC.

### 7. Reload Semantics Need Guardrails

You list this as an open question; I’d suggest a defined “safe default”:
- reload applies to *future* scheduling and routing.
- in-flight jobs continue with the config snapshot they started with.
- config reload should validate fully before swap; if invalid, keep old config.

Define what happens if a plugin disappears or is disabled during reload:
- do queued jobs get canceled, or continue? I’d default to continue, but stop enqueueing new ones.

### 8. Plugin Trust Model and Execution Hardening

Because plugins are executables, the gateway is effectively an execution engine. Even for personal use, accidental damage is easy (e.g., wrong script permissions, path traversal, editing run.sh).

Suggested minimal constraints:
- plugins must live under `plugins_dir` and be executed via absolute path resolution.
- require `manifest.yaml` and explicit `entrypoint` to avoid guessing `run.py` vs `run.sh`.
- refuse world-writable plugin dirs.
- run as a dedicated OS user (you already show systemd user/group).

## “Over-Engineered?” vs “Under-Specified?”

Nothing here is over-engineered for the goal; the risk is actually the opposite: key semantics are under-specified, and those semantics are where integration systems fail.

If you want to keep it minimal, I’d still insist on:
- retries/backoff + timeouts,
- job state transitions,
- protocol versioning and error contract,
- webhook verification and request limits.

Everything else (parallelism, DAG visualization, UI, etc.) can remain out-of-scope.

## Concrete Suggestions to Add to the RFC

### Add a “Semantics” Section

Define:
- delivery guarantee: **at-least-once**,
- plugin responsibility: idempotency and dedupe using state,
- core responsibility: retries/backoff, dead-lettering, observability.

### Expand Job Model

Current: `{id, plugin, command, payload, priority, submitted_by, created_at}`

Recommend:
- `status`, `attempt`, `max_attempts`, `next_run_at`,
- `lease_expires_at` (or `heartbeat_at`) for crash recovery,
- `dedupe_key` (optional),
- `parent_job_id` and/or `source_event_id` (for provenance).

### Add Retry/Backoff Defaults

Example:
- `max_attempts: 5`
- exponential backoff with jitter, cap at 30m
- non-retryable error codes for auth failures vs transient HTTP failures.

### Specify State Value Format

`plugin_state` as `(plugin_name, key, value)` implies string values. That’s fine, but define it:
- value stored as JSON text, or
- provide `type` column, or
- constrain to JSON object per plugin (single row).

I’d pick “single JSON blob per plugin” unless you expect lots of independent keys.

### Define Logging and Redaction

Decide:
- core logs in JSON, include `job_id`, `plugin`, `command`.
- plugin stdout reserved for protocol; plugin stderr is captured and attached to job logs (capped).
- redact common secret patterns and configured keys.

### Routing Semantics

Define:
- fan-out behavior (multiple matching routes).
- what happens when no route matches (drop? log?).
- event type namespace rules (string naming, versioning).
- optional filters (e.g., route only when `payload.severity == "high"`).

## Smaller Nits (Worth Fixing in the Draft)

- Clarify whether “scheduler checks due plugins” uses “last successful run” vs “last attempted run”.
- “Jitter: each cycle picks a random offset” can create uneven behavior; I’d specify jitter per *scheduled run time* rather than per tick, so runs don’t “wander.”
- Plugin `manifest.yaml` has `config_keys` but the RFC doesn’t say whether the core validates required keys or just passes config through.
- `ductile reload` uses SIGHUP in systemd; the CLI command is shown but not how it signals the daemon (PID file? unix socket? HTTP admin endpoint?).
- For `webhooks.listen: 127.0.0.1:8081`, call out the intended deployment model (reverse proxy? SSH tunnel? local-only triggers?).

## Questions I’d Want Answered Before Implementation

1. What’s the minimum acceptable guarantee for actions like “notify”: can it double-send? If not, where does dedupe live (plugin state vs core)?
2. Are plugin outputs “events” only, or can a plugin also directly enqueue specific downstream commands (I recommend keeping it events-only, but clarify)?
3. Do you want the core to help with OAuth refresh (shared helpers), or is it strictly plugin-owned?
4. What’s the expected upper bound of payload size for events and webhook bodies?
5. Will you ever run more than one instance (even accidentally)? If yes, implement a single-instance lock early.

## Suggested Acceptance Criteria (Helps Keep Phase 2 Honest)

- A hung plugin cannot stall the gateway indefinitely (timeouts enforced).
- Restart recovers queued/running work deterministically (no permanent “running” jobs).
- A plugin/core protocol mismatch fails fast with a clear error (versioning).
- A transient network failure causes a retry with backoff (and visible in `job_log`).
- Webhook verification rejects invalid signatures and oversized requests.
