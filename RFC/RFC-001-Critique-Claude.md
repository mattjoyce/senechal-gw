# RFC-001 Critique: Senechal Gateway

**Reviewer:** Claude (team review)
**Date:** 2026-02-08

---

## Overall Impression

This is a well-structured RFC with clear rationale. The design sits in a good sweet spot — lighter than n8n/Huginn but more capable than shell scripts and cron. The work queue as central abstraction is the right call, and the subprocess plugin model is pragmatic. My critique focuses on areas where the design could be tightened or where I see risks hiding.

---

## What Works Well

- **Work queue as the unifying abstraction** — This is the strongest decision in the doc. Having scheduler, webhooks, CLI, and plugin output all feed into the same queue means you only need to get dispatch right once. It also gives you free observability (queue depth, job history) without extra plumbing.

- **Plugins as subprocesses** — Perfect for a personal server. Process isolation means a buggy Python plugin can't crash the core. Language-agnostic means you can port existing Senechal code directly.

- **Config-declared routing** — Keeping plugins dumb and putting routing in config is the right separation. Plugins don't need to know about each other.

- **No crontab syntax** — `every: 6h` with `jitter: 30m` is immediately readable. Good call.

---

## Substantive Concerns

### 1. Serial Dispatch May Bite Sooner Than Expected

Serial execution is fine for polling plugins that complete in seconds. But consider: a Withings OAuth token refresh times out (30s), and behind it in the queue are 5 Google Calendar polls and a webhook-triggered notification. That notification now waits potentially minutes.

**Suggestion:** Keep serial as the default, but consider a simple priority lane — at minimum, separate webhook-triggered jobs (which have latency expectations from the sender) from scheduled polls (which don't). Even two lanes (urgent + normal) with serial execution within each lane would help. This doesn't require full concurrency — just two goroutines pulling from two priority levels.

### 2. The Plugin Lifecycle Model Is Underspecified

The protocol shows four commands (`poll`, `handle`, `health`, `init`) but doesn't describe:

- **When does `init` run?** On every invocation? First run only? After config changes?
- **Is the plugin process long-lived or spawn-per-command?** The doc says "spawns them as subprocesses" which implies spawn-per-command, but this should be explicit. Spawn-per-command is simpler and safer. Long-lived means managing process health, restarts, and stdin/stdout buffering.
- **What does `health` return?** Just `{"status": "ok"}`? Or should it report capability, readiness, dependency checks?

**Suggestion:** Be explicit that plugins are **spawn-per-command** (one process per job). State this as a design principle. It eliminates an entire class of problems (leaked connections, memory growth, zombie processes). The overhead of process spawn is negligible for personal-use intervals.

### 3. Error Handling and Retry Strategy Is Missing

The RFC doesn't address what happens when a plugin returns an error or a non-zero exit code. Questions:

- Does a failed job get retried? How many times? With backoff?
- Does a failed job block downstream routes?
- Where do failed jobs go — back in the queue, into a dead-letter table, just logged?
- If a plugin consistently fails, does the scheduler keep enqueuing new jobs for it? (This could fill the queue with doomed work.)

**Suggestion:** Add a simple retry policy to the job model: `max_retries` (default 3), exponential backoff, and a `failed` status that stops retries. A circuit breaker per plugin (stop scheduling after N consecutive failures, auto-reset after a cooldown) would prevent queue flooding. This is important enough to belong in Phase 2, not deferred.

### 4. The Event Schema Needs Definition

Routes reference `event_type: new_health_data` and `event_type: alert`, but the `events` array in the plugin response has no defined schema. What does an event look like?

```json
{"type": "new_health_data", "payload": {"weight": 82.1}}
```

or something richer? Without this, plugin authors will invent incompatible formats.

**Suggestion:** Define a minimal event envelope early:

```json
{
  "type": "string (matches route event_type)",
  "payload": {},
  "timestamp": "ISO8601",
  "source": "plugin_name (injected by core)"
}
```

### 5. Webhook Security Model Is Thin

The config shows `secret: ${GITHUB_WEBHOOK_SECRET}` but doesn't specify:

- How the secret is verified (HMAC-SHA256 signature check? Bearer token? Both?)
- Whether the webhook endpoint is authenticated at all beyond the path being obscure
- Rate limiting or request size limits

For a service listening on a port — even localhost — this matters. If you ever expose it via a reverse proxy, you want the core to validate signatures, not rely on network-level security alone.

**Suggestion:** Build HMAC signature verification into the webhook handler (it's ~20 lines of Go). Document the supported schemes. Default to rejecting requests without valid signatures.

### 6. Plugin Config vs. Plugin State Boundary

The plugin receives both `config` and `state` on every invocation, and returns `state_updates`. But the line between config and state isn't clear for things like OAuth tokens:

- `access_token` is listed as a `config_key` in the manifest, but tokens expire and need refreshing.
- If the plugin refreshes the token and returns it as a `state_update`, then the "real" token lives in state, not config. But if the config also has `access_token`, which wins?

**Suggestion:** Pick one of two models and be explicit:
1. **Config is static, state is dynamic.** OAuth tokens live in state. Plugins handle refresh internally and persist new tokens via `state_updates`. Config only holds `client_id`/`client_secret`.
2. **Core manages OAuth.** The core handles token refresh (it already knows the client credentials) and injects fresh tokens into config. Plugins never see refresh logic.

Option 1 is simpler to build. Option 2 is better long-term (avoids duplicating OAuth flows across plugins). For a personal server, start with option 1 and document it.

---

## Smaller Notes

- **Plugin stderr** (from Open Questions): Capture it. Always. Route it to structured logs tagged with the plugin name. This is your primary debugging tool when a Python plugin throws a traceback. It's trivial to implement and invaluable when things break at 3am.

- **Plugin timeouts** (from Open Questions): Default 30s, configurable per-plugin in config. Kill the process on timeout. The job gets a `timeout` status and follows the retry policy. Non-negotiable for a system that runs unattended.

- **Multi-instance safety** (from Open Questions): SQLite's WAL mode with `PRAGMA busy_timeout` handles this well enough. But simpler: write a PID file on startup, check it, and refuse to start if another instance is running. Classic, reliable, zero-complexity.

- **Config reload** (from Open Questions): Let in-flight jobs finish with the old config. New config applies to newly enqueued jobs only. Don't try to hot-swap mid-execution — it's not worth the complexity.

- **`job_log` table will grow forever** — Add a retention policy (e.g., keep 30 days, or last N entries per plugin). A single `DELETE FROM job_log WHERE completed_at < ?` on each tick is fine. Without this, the SQLite file will grow unbounded on a long-running personal server.

- **No health endpoint for the core itself** — If you're running this under systemd, consider a simple `/healthz` on the webhook listener (or a separate port). Useful for monitoring and for systemd's `Type=notify` or watchdog integration.

- **`ExecReload=/bin/kill -HUP $MAINPID`** — Make sure the Go binary actually handles SIGHUP for config reload. Easy to forget this side of the contract.

- **Missing from project layout:** a `testdata/` or `fixtures/` directory. Plugin protocol testing will want sample request/response JSON files.

---

## Answers to Your Specific Questions

> 1. Is the work queue as central abstraction the right model?

**Yes.** It's the strongest part of the design. It unifies all trigger types, gives you crash recovery for free, and makes the system observable. Don't second-guess this one.

> 2. Is subprocess (JSON over stdin/stdout) the right plugin boundary?

**Yes, with the caveat that you should explicitly commit to spawn-per-command.** The alternative (gRPC, HTTP sidecar) adds operational complexity that isn't justified for a personal server. JSON-over-stdio is debuggable with `echo '{}' | ./run.py` — that's a feature.

> 3. Is Go the right choice for the core, given polyglot plugins?

**Yes.** Single binary deployment, good subprocess management (`os/exec`), built-in HTTP server for webhooks, SQLite via `mattn/go-sqlite3` or `modernc.org/sqlite` (pure Go, no CGO). The core doesn't need to be in the same language as the plugins — that's the whole point of the subprocess boundary.

> 4. What's missing from this design?

The biggest gaps: error/retry strategy, plugin lifecycle specifics, and event schema. All addressed above. Also missing but lower priority: log rotation strategy, graceful shutdown semantics (drain queue vs. kill immediately), and a plugin development guide (how to test a plugin locally without the full core).

> 5. What's over-engineered for a personal integration server?

Honestly, not much — this is lean. If anything, `preferred_window` on scheduling is a nice-to-have that could be deferred. And `priority` on jobs adds complexity to queue ordering that you may never use. Start without priority (FIFO) and add it when you have a real need.

---

## Summary

This is a solid design for a personal integration gateway. The core abstractions (queue, subprocess plugins, config routing) are right. The main gaps are in the operational details — error handling, retries, timeouts, and plugin lifecycle — which is normal for a first-draft RFC. Address those before Phase 2 and you'll have a system that runs reliably unattended, which is the real test of a personal server.

Ship it (after addressing the error handling gap).
