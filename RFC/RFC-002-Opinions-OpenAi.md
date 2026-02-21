# RFC-002 Opinions (OpenAI) — Lock The Design

**Status:** Historical Critique
Target: `RFC-002-operational-semantics.md` (Draft, dated 2026-02-08)

This RFC is ~90% of the way to “buildable”. The overall direction is correct and matches the product constraint (personal gateway, unattended reliability, low complexity).

I’m taking a stance: **ship this design with a small set of binding tweaks** (below). Don’t broaden scope; just close the few gaps that will otherwise create “haunted gateway” failure modes (duplicate polls, runaway queues, un-debuggable drops).

## Lock These Decisions (No Bikeshedding)

These are the right defaults for V1 and should be treated as final:

1. **At-least-once delivery** (Decision 1). Correct tradeoff. Push idempotency to plugins; keep core generic.
2. **Serial, single-lane FIFO dispatch** (Decision 14). Correct for a personal server. Concurrency is a distraction until metrics force it.
3. **Spawn-per-command** (Decision 5). Strong choice. It eliminates the majority of “daemon management” complexity.
4. **Hard timeouts with kill escalation** (Decision 4). Non-negotiable and correctly specified.
5. **Versioned request/response envelope, stdout reserved for protocol** (Decision 7 + Decision 18). This is the protocol contract that prevents drift.
6. **SQLite plugin state as one JSON blob per plugin** (Decision 9). Pragmatic and sufficient; avoids premature schema work.
7. **OAuth is plugin-owned** (Decision 10). Correct boundary.
8. **Webhook HMAC verification + body size caps** (Decision 11). Enough for V1 given localhost binding.
9. **`flock` single-instance lock** (Decision 12). Correct and simple.
10. **Reload semantics: validate then swap, in-flight jobs continue** (Decision 13). Safe operational behavior.
11. **Routing: fan-out, exact match, no filters in V1** (Decision 15). Keeps router dumb and predictable.
12. **Scheduler jitter per scheduled run (no wander) and preferred_window snap** (Decision 17). Correct behavior.
13. **Time-based job log retention** (Decision 19). Boring and good.
14. **/healthz** (Decision 20). Useful and low-risk.

If you feel tempted to revisit any of these, don’t. Build the system first; measure; revisit later if reality forces it.

## Binding Changes Before Implementation (Must Fix)

These are small, but they matter enough that I’d block implementation until they’re settled in the RFC.

### 1. Fix the stderr/stdout cap inconsistency

Decision 4 says `stderr` cap is **1 MB**; Decision 18 says `stderr` stored cap is **64 KB** (and Decision 4 also mentions truncation behavior).

Pick one policy and state it clearly. My recommendation:

- **Capture caps (in-memory while the process runs):**
  - `stdout`: 10 MB (protocol JSON only)
  - `stderr`: 1 MB
- **Persistence caps (what gets stored in SQLite `job_log`):**
  - `stderr`: 64 KB (truncate with “truncated” flag/bytes_dropped)
  - `stdout`: store none or store only on protocol error (capped), since stdout is reserved for protocol

Rationale: you want enough stderr to debug, but you don’t want DB bloat or accidental secret spillage.

### 2. Dedupe semantics: do not “silently drop” without observability

Decision 1 says: if `dedupe_key` matches a job that succeeded within 24 hours, the job is silently dropped.

That’s operationally dangerous because it creates invisible behavior (“why didn’t my job run?”). Lock this instead:

- If a producer enqueues with `dedupe_key` that matches a recent successful job (within `dedupe_ttl`), **do not enqueue**, but:
  - log at `INFO` with `dedupe_key` and the existing job id
  - optionally record a lightweight `dedupe_hit` row or counter for observability

Also: make the **24h TTL configurable** (`service.dedupe_ttl` default 24h), because some workflows are weekly/monthly.

### 3. Prevent “scheduler spam”: only one outstanding scheduled poll per plugin

Right now, nothing prevents the scheduler from enqueuing multiple `poll` jobs for the same plugin while prior attempts are failing/retrying, which can create an unbounded backlog (especially with serial dispatch).

Binding rule:

- For scheduled `poll` jobs: the scheduler **must not enqueue** a new `poll` if there is already a `queued` or `running` `poll` job for that plugin.

Implementation-friendly options:

- Enforce in scheduler logic via a query.
- Or enforce via a uniqueness constraint (e.g., “at most one queued/running poll per plugin”) if you prefer DB-level invariants.

This single rule prevents the most common runaway behavior in personal schedulers.

### 4. Circuit breaker scope: apply it to scheduling, not dispatching queued work

Decision 3 says: circuit breaker stops enqueuing new jobs, but existing queued jobs still execute. That can undermine the breaker (you can still churn on failures for a long time).

Binding stance:

- Circuit breaker should primarily protect **the scheduler** (stop creating new scheduled `poll` jobs).
- It should **not** block webhook-driven `handle` jobs by default (those are externally triggered and might be urgent), but you should at least:
  - track consecutive failures per `(plugin, command)` so a failing `poll` doesn’t suppress `handle` (and vice versa)

If you want to keep it even simpler: make the breaker explicitly “scheduler-only” and say so in the RFC.

### 5. Add a core-assigned event id for traceability

Events currently have `{type, payload, dedupe_key?}` and the core injects source/timestamp when creating downstream jobs. You’re missing a stable identifier for correlation across logs and for debugging “what produced this?”

Binding addition:

- Core assigns `event_id` when processing plugin response.
  - Suggested: `event_id = uuid()` OR `event_id = hash(job_id + event_index)` (deterministic is fine)
- Downstream job records `source_job_id` (already have `parent_job_id`; use it consistently) and `source_event_id`.

This isn’t “extra”; it’s the minimum to debug fan-out systems.

## Minor Tweaks I’d Make Now (Low Risk, High Clarity)

These aren’t blockers, but they tighten the spec without expanding scope:

- **State updates semantics** (Decision 9): keep shallow-merge, but explicitly document “nested updates require replacing the top-level key” and add an escape hatch:
  - allow response field `"state_replace": true` to replace the whole blob (rare, but useful)
- **Retryability by exit code** (Decision 3): you already reserve `78` for non-retryable config errors. Consider also treating `EX_TEMPFAIL (75)` as explicitly retryable and documenting it (optional).
- **Non-streaming protocol** is fine. Just explicitly say: “plugins must finish within timeout; no long-lived listeners in V1.”

## Defer (Do Not Add To V1)

Keep these out of the first build. They’re tempting, but they’re scope traps:

- Priority queues / multi-lane dispatch
- Router query language / payload filters
- OAuth in the core
- Webhook replay protection / timestamp verification (unless a specific provider forces it)
- Secret redaction in core logs (operator responsibility is acceptable for V1)
- Streaming / long-lived plugin mode (if you ever need WebSockets, it’s a different class of system)

## Bottom Line

Treat RFC-002 as the design, **apply the 5 binding changes above**, and start implementing. This locks a coherent V1 that will behave predictably under crash/retry/timeout, without overbuilding.
