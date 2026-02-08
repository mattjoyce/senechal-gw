# RFC-002: Operational Semantics — Consensus Review

**Status:** Consensus
**Date:** 2026-02-08
**Sources:** Claude (Opus 4.6), OpenAI, Gemini
**Covers:** RFC-002-operational-semantics.md

---

## Verdict: Accept with Binding Amendments

All three reviewers independently endorse RFC-002 as a sound, buildable design. The core philosophy — spawn-per-command, push complexity to plugins, serial FIFO dispatch, at-least-once delivery — received unanimous strong approval. No reviewer suggested architectural changes; disagreements are limited to parameter tuning and small specification gaps.

---

## Unanimous Agreements (No Debate)

The following decisions received unqualified acceptance from all three reviewers. These should be treated as final and locked:

| # | Decision | Notes |
|---|----------|-------|
| 1 | At-least-once delivery | Correct tradeoff; `dedupe_key` is the right escape hatch |
| 2 | Job state machine | Clean, no ambiguous transitions; removing `priority` was correct |
| 4 | Plugin timeouts with SIGTERM/SIGKILL escalation | Non-negotiable; defaults are sensible |
| 5 | Spawn-per-command lifecycle | **Keystone decision** — eliminates entire classes of operational complexity |
| 6 | Crash recovery via lock + orphan detection | Simple, elegant, correct |
| 8 | Event envelope | Minimal and sufficient; core-injected `source`/`timestamp` is right |
| 9 | Plugin state as single JSON blob, shallow merge | Correct; avoids premature schema work |
| 10 | OAuth is plugin-owned | Unanimous strong agreement; core must not understand OAuth |
| 11 | Webhook HMAC-SHA256 verification | Correct for V1; replay protection and rate limiting deferred appropriately |
| 12 | PID file with `flock` | Classic, zero-dependency, kernel handles crash cleanup |
| 13 | Config reload via SIGHUP | Standard Unix practice; in-flight jobs keep old config |
| 14 | Serial, single-lane FIFO dispatch | Correct for scale; resist the urge to add concurrency prematurely |
| 15 | Routing: fan-out, exact match, no filters | Aggressively simple and correct for V1 |
| 16 | Plugin trust & execution constraints | Sound baseline security |
| 17 | Jitter per-run, preferred_window as hard constraint | Prevents schedule wander; correct detail |
| 18 | Logging: JSON to stdout, stderr captured | Clear separation of concerns |
| 19 | Job log retention: 30 days, time-based | Simple and adequate for expected volume |
| 20 | Core `/healthz` endpoint | Essential operational feature |

---

## Binding Amendments (Must Address Before Implementation)

These issues were raised by one or more reviewers with sufficient justification to warrant resolution before coding begins.

### Amendment 1: Fix stderr/stdout cap inconsistency

**Raised by:** OpenAI
**Issue:** Decision 4 specifies stderr capture cap of **1 MB**; Decision 18 specifies stored stderr cap of **64 KB**. These are not contradictory but the RFC doesn't distinguish capture vs persistence.

**Consensus resolution:** Adopt a two-tier model:
- **Capture caps** (in-memory while process runs): stdout 10 MB, stderr 1 MB
- **Persistence caps** (stored in SQLite `job_log`): stderr 64 KB (truncated with flag), stdout stored only on protocol error (capped at 64 KB)

State both explicitly in the RFC.

### Amendment 2: Circuit breaker threshold — lower from 5 to 3

**Raised by:** Claude (lower to 3), OpenAI (scope to scheduler-only)
**Gemini position:** 5 is well-balanced as-is.

**Consensus resolution:** Lower to **3 consecutive failures**. Claude's reasoning is persuasive: 5 failures x 4 attempts each = 20 failed spawns before the circuit opens, which is excessive noise for a personal server. Additionally, per OpenAI's point, scope the breaker to **scheduler-originated poll jobs** — webhook-triggered `handle` jobs should not be blocked by poll failures. Track failures per `(plugin, command)` pair.

### Amendment 3: Add `protocol` field to response envelope

**Raised by:** Claude
**Issue:** Only the request includes `protocol: 1`. The response should echo it back for bidirectional version detection during future protocol upgrades.

**Consensus resolution:** Add `"protocol": 1` to the response envelope. Zero-cost addition that prevents debugging headaches later.

### Amendment 4: Dedupe drops must be observable

**Raised by:** OpenAI
**Issue:** Decision 1 says deduped jobs are "silently dropped." Silent drops create invisible behavior that is difficult to debug.

**Consensus resolution:**
- When a job is deduped, log at `INFO` with the `dedupe_key` and the ID of the existing successful job.
- Make the 24h TTL configurable via `service.dedupe_ttl` (default 24h) to support weekly/monthly workflows.

### Amendment 5: Prevent scheduler poll spam

**Raised by:** OpenAI
**Issue:** Nothing prevents the scheduler from enqueuing multiple `poll` jobs for the same plugin while prior polls are still queued/running/retrying.

**Consensus resolution:** The scheduler **must not enqueue** a new `poll` if there is already a `queued` or `running` `poll` job for that plugin. Enforce via scheduler query before enqueue. This prevents unbounded backlog under serial dispatch.

### Amendment 6: Add event_id for traceability

**Raised by:** OpenAI
**Issue:** Events lack a stable identifier for correlating across fan-out chains and debugging.

**Consensus resolution:** Core assigns `event_id` (UUID) when processing plugin response events. Downstream jobs record both `parent_job_id` (already exists) and `source_event_id`. This is minimum viable traceability for fan-out debugging.

---

## Minor Recommendations (Non-Blocking)

These were raised by individual reviewers and are worth considering but should not block implementation.

| Recommendation | Source | Notes |
|----------------|--------|-------|
| Add `retry` as a `submitted_by` value for re-queued jobs | Claude | Cosmetic; improves log clarity. Can be added later without schema change. |
| Document that OAuth `state_updates` must include all token fields atomically | Claude | Plugin documentation concern, not an RFC change. |
| Add `state_replace: true` escape hatch for full state blob replacement | OpenAI | Rare but useful. Low risk to add. |
| Treat exit code `75` (EX_TEMPFAIL) as explicitly retryable | OpenAI | Optional; aligns with sysexits.h conventions. |
| Never add configurable redaction patterns | Claude | Strong opinion: redaction patterns create false security. Fix plugins instead. |

---

## Consensus on Feedback Questions

### Q1: At-least-once vs per-plugin at-most-once?
**Unanimous:** At-least-once for the whole system. One mental model for plugin authors. Plugins that cannot tolerate duplicates use `dedupe_key` or track in state. Do not split the guarantee.

### Q2: Circuit breaker tuning?
**Majority (2/3):** Too lenient at 5. Lower to 3. Scope to scheduler-originated jobs. (See Amendment 2.)

### Q3: Spawn-per-command for persistent connections?
**Unanimous:** No. Persistent connections (WebSockets, long-polling) are fundamentally incompatible with spawn-per-command. They are out of scope. If needed, run them as separate services that push events into Senechal via the webhook endpoint.

### Q4: Streaming plugin mode?
**Unanimous:** Out of scope. Not now, not ever for this core. If streaming is needed, use an external system that Senechal's spawn-per-command plugins interact with. The plugin model's value is simplicity through ephemeral processes. A streaming mode would destroy that.

---

## Author Disposition

| # | Amendment | Author Decision |
|---|-----------|-----------------|
| A1 | Clarify two-tier stderr/stdout caps (capture vs persistence) | **Deferred** — later decision |
| A2 | Circuit breaker: 5 → 3; scope to (plugin, command); scheduler-only | **Accepted** — make threshold configurable (default 3) |
| A3 | Add `protocol` field to response envelope | **Deferred** — later decision |
| A4 | Log deduped jobs at INFO; make dedupe_ttl configurable | **Accepted** |
| A5 | Scheduler must not enqueue duplicate polls per plugin | **Accepted** — make configurable (e.g. `max_outstanding_polls`, default 1) |
| A6 | Core assigns `event_id`; downstream jobs record `source_event_id` | **Accepted** — implement unless it creates regret work; drop if it forces premature schema commitments |

### Summary of V1 implementation scope

**Build now:**
- A2: Configurable circuit breaker threshold (default 3), scoped per (plugin, command), scheduler-only for polls
- A4: Deduped jobs logged at INFO; `service.dedupe_ttl` configurable (default 24h)
- A5: Configurable max outstanding polls per plugin (default 1)
- A6: `event_id` on events and `source_event_id` on downstream jobs — accept unless schema cost is disproportionate

**Deferred (revisit post-V1):**
- A1: Two-tier capture vs persistence caps — current spec is workable, clarify later
- A3: `protocol` field in response envelope — can be added in protocol v2 without breaking anything
