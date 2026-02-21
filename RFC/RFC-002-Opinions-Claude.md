# RFC-002 Review: Operational Semantics

**Status:** Historical Critique
**Reviewer:** Claude (Opus 4.6)
**Date:** 2026-02-08
**Verdict:** Accept with minor amendments. Ship it.

---

## Overall Assessment

This RFC is unusually good for a V1 spec. It makes 20 decisions and gets 17 of them completely right on the first pass. The remaining 3 need tweaks, not redesigns. The consistent theme — "this is a personal server, not a distributed system" — is the correct framing and it's applied consistently. Build against this.

---

## Decision-by-Decision

### 1. At-Least-Once Delivery — ACCEPT

Correct. No notes. The dedupe_key mechanism is the right escape hatch. Plugins that can't tolerate duplicates use it; plugins that don't care ignore it.

The 24-hour dedupe window is fine. Don't overthink it.

### 2. Job State Machine — ACCEPT

Clean state machine with no ambiguous transitions. Removing `priority` was the right call — priority systems in small-scale systems always become "everything is high priority" within a month.

One small thing: the `submitted_by` field should include `retry` as a value for re-queued jobs, so you can distinguish "scheduler created this" from "retry logic re-queued this" in logs. Not blocking — you can add it later without schema changes since it's just a string.

### 3. Retry & Backoff — ACCEPT

Exit code 78 for non-retryable failures is clever and correct — it's a well-known convention that won't surprise anyone who's written Unix software.

The circuit breaker at 5 consecutive failures / 30-minute reset is **too lenient** for a personal server. Here's why: if a plugin is broken (bad credentials, API decomissioned), 5 failures × 4 attempts each = 20 failed process spawns before the circuit opens. That's noise you don't need. **Change to 3 consecutive failures.** The 30-minute reset is fine — it gives APIs time to recover from transient outages.

### 4. Plugin Timeouts — ACCEPT

The timeout hierarchy (poll 60s, handle 120s, health 10s, init 30s) is sensible. The SIGTERM → 5s grace → SIGKILL escalation is textbook correct.

The 10 MB stdout cap is generous but fine. You'll never hit it with well-behaved plugins, and truncation with a warning is the right response for misbehaving ones.

### 5. Spawn-Per-Command — ACCEPT, this is the keystone decision

This is the single most important decision in the RFC and it's correct. Every operational complexity question (lifecycle management, memory leaks, zombie processes, state corruption) evaporates with spawn-per-command.

Re: feedback question 3 (WebSocket listeners) — **WebSocket listeners are out of scope. Period.** A WebSocket listener is a long-lived service, not a plugin. If you need one, run it as a separate process that pushes events into Ductile via the webhook endpoint. Don't contaminate the plugin model. This answers feedback question 4 too: no streaming plugin mode. Ever. If something needs to be long-lived, it's not a plugin — it's a service that talks to Ductile via HTTP.

### 6. Crash Recovery — ACCEPT

Simple and correct. The advisory lock + "everything running is orphaned" logic is exactly right. No heartbeat files, no stale timeout tuning. The kernel handles liveness; the core handles recovery. Clean separation.

### 7. Protocol Specification — ACCEPT with one amendment

The protocol is clean. Single JSON object in, single JSON object out, process exits. Good.

**Amendment:** Add a `version` field to the response envelope too. Right now only the request has `protocol: 1`. The response should echo it back: `"protocol": 1`. This costs nothing and makes it trivial to detect version mismatches in both directions during a future protocol upgrade. Without it, you'll be guessing whether a malformed response came from a v1 or v2 plugin.

### 8. Event Envelope — ACCEPT

Minimal and correct. The core injecting `source` and `timestamp` is the right division of responsibility.

### 9. Plugin State Model — ACCEPT

Single JSON blob per plugin, shallow merge on updates. This is the right call. Key-value state stores always end up needing transactions across keys, and then you've reinvented a document store badly.

The 1 MB limit is appropriate. If a plugin needs more, it should write to a file — and honestly, if a plugin's state exceeds 1 MB, that's a design smell in the plugin.

### 10. OAuth: Plugin-Owned — ACCEPT

Absolutely correct. OAuth is too provider-specific for the core to own. The `plugins/lib/` suggestion for shared helpers is the right level of reuse.

One operational note worth adding to documentation (not the RFC): remind plugin authors that `state_updates` for token refresh must include **all** token fields atomically. If a plugin returns `{"access_token": "new"}` but forgets `{"token_expiry": "..."}`, the stale expiry will cause an immediate re-refresh on the next invocation. This is a plugin bug, not a core concern, but it will be the #1 OAuth debugging issue.

### 11. Webhook Security — ACCEPT

HMAC-SHA256, configurable header name, no error details in rejection response. Correct on all counts.

No replay protection in V1 is the right call. Replay protection is provider-specific and the threat model for a localhost-bound personal server doesn't justify it.

### 12. Multi-Instance Lock — ACCEPT

PID file with `flock`. Classic, correct, zero-dependency. Nothing to add.

### 13. Config Reload — ACCEPT

SIGHUP reload with "in-flight jobs keep old config" is the only sane behavior. The cancel-on-removal semantics for removed/disabled plugins are correct.

### 14. Serial, Single Lane — ACCEPT, and resist the urge to revisit

This will feel wrong to anyone with distributed systems experience. It is correct for this system. 50 jobs/day with a median runtime of maybe 2 seconds each. Serial FIFO eliminates an entire category of concurrency bugs (race conditions on state updates, resource contention, ordering violations).

The stated revisit condition (500 jobs/day or 30s median wait) is reasonable. I'd go further: **don't revisit until someone is actually complaining about latency with data to back it up.** Not "this might be slow" — "this IS slow and here are the numbers."

### 15. Routing Semantics — ACCEPT

Fan-out with exact string matching. No wildcards, no regexes, no conditional filters. This is aggressively simple and that's exactly right for V1.

The "put conditional logic in the plugin" guidance is correct. A plugin that receives an irrelevant event and no-ops is trivially cheap (spawn, read stdin, write `{"status":"ok"}`, exit). Don't build a query language to avoid a 10ms no-op.

### 16. Plugin Trust & Execution — ACCEPT

Symlink resolution, path traversal rejection, world-writable check. Good baseline security without overengineering.

### 17. Jitter Behavior — ACCEPT

Per-run jitter with `preferred_window` as a hard constraint is correct. The "no wander" property (jitter is fixed for a scheduled run, not re-randomized per tick) shows attention to detail.

### 18. Logging & stderr — ACCEPT

JSON logs to stdout, stderr captured and stored. The 64 KB cap on stored stderr is fine.

**One strong opinion:** Do not add configurable redaction patterns later. Ever. Redaction patterns create a false sense of security ("I added a regex for API keys, so I'm safe") while missing novel secret formats. The correct approach is: don't log secrets. If a plugin logs secrets to stderr, that's a plugin bug. Fix the plugin, don't bandage the core.

### 19. Job Log Retention — ACCEPT

30 days, time-based, simple `DELETE`. At 50 jobs/day this is trivial. The temptation to add per-plugin retention or row-count limits should be resisted.

### 20. Core Health Endpoint — ACCEPT

`/healthz` with queue depth and circuit breaker status. Enough for systemd watchdog integration and quick operator checks. Don't add more fields until someone asks for them.

---

## Responses to Feedback Questions

1. **At-least-once vs at-most-once per plugin?** No. One delivery guarantee for the whole system. Plugin authors need exactly one mental model. If a specific plugin truly cannot tolerate duplicates, it uses `dedupe_key` or tracks in state. Don't split the guarantee.

2. **Circuit breaker too aggressive or too lenient?** Too lenient. Change from 5 to 3 consecutive failures. See Decision 3 notes above.

3. **Spawn-per-command viable for persistent connections?** Wrong question. Persistent connections are not plugins. They're separate services that push events into Ductile via webhooks. Don't bend the plugin model.

4. **Streaming plugin mode?** No. Not now, not ever. If it needs to stream, it's not a plugin. The plugin model's entire value proposition is simplicity through spawn-per-command. A "streaming mode" destroys that.

---

## Summary of Recommended Changes

| Decision | Change | Severity |
|----------|--------|----------|
| 3 | Circuit breaker: 5 → 3 consecutive failures | Minor |
| 7 | Add `protocol` field to response envelope | Minor |
| 2 | Add `retry` as a `submitted_by` value (optional, can defer) | Cosmetic |

Everything else: build as specified. Stop designing. Start coding.
