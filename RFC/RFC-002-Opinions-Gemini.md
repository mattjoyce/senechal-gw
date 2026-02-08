# RFC-002: Operational Semantics — Gemini Critique

**Status:** Critique
**Date:** 2026-02-08
**Author:** Gemini CLI Agent
**Critiques:** RFC-002-operational-semantics.md

---

## Overall Stance: Strong Endorsement

This RFC makes 20 highly pragmatic, well-reasoned decisions that successfully balance robustness, simplicity, and necessary functionality for a personal integration gateway. The core philosophy of "spawn-per-command" and pushing complexity to the plugins (where it belongs) is consistently applied. This design effectively avoids the "class of bugs that make integration gateways feel haunted."

The RFC clearly articulates compromises (e.g., at-least-once, serial dispatch) based on the stated scope ("personal server, maybe 50 jobs/day"), which is a hallmark of good design. Perfection truly is the enemy of good enough here, and this RFC lands firmly in "good enough" territory, providing a solid foundation.

---

## Detailed Decisions Critique

### 1. Delivery Guarantee: At-Least-Once
**Opinion:** **Agree.** This is the correct, pragmatic choice for the architecture. Achieving exactly-once delivery in a subprocess-based system is prohibitively complex and unnecessary for this scope. The `dedupe_key` mechanism provides a sensible, opt-in layer of deduplication for common scenarios, while correctly delegating more complex, application-specific idempotency to the plugins.

### 2. Job State Machine
**Opinion:** **Agree.** The proposed state machine is comprehensive and correctly captures all necessary transitions for robust job management, including retries and terminal states. The expanded job model provides essential metadata for debugging and operational visibility. The removal of `priority` simplifies the core, aligning with the "personal server" scope where FIFO dispatch is perfectly acceptable.

### 3. Retry & Backoff
**Opinion:** **Agree.** The use of exponential backoff with jitter is a proven, resilient strategy for retries. The default `max_attempts` (4 total) is a reasonable balance between persistence and preventing indefinite retries for persistently failing jobs. The `EX_CONFIG` condition for non-retryable failures is a critical detail, preventing system resources from being wasted on unfixable misconfigurations. The circuit breaker is an excellent addition, providing essential self-preservation for the core when a plugin becomes unstable.

### 4. Plugin Timeouts
**Opinion:** **Strongly Agree.** Timeouts are absolutely non-negotiable. Hung plugins are a primary source of "haunted gateway" scenarios. The `SIGTERM`/`SIGKILL` sequence with a grace period is the standard and correct approach for process control. Sensible default timeouts, combined with per-plugin configurability, provide the right balance of safety and flexibility. Resource caps (stdout/stderr) are also wise precautions against runaway processes.

### 5. Plugin Lifecycle: Spawn-Per-Command
**Opinion:** **Strongly Agree.** This is arguably the most fundamental and robust design decision in the RFC. It eliminates an entire class of complex problems associated with long-lived processes (heartbeats, memory leaks, connection management, state synchronization) by embracing the ephemeral nature of subprocesses. The argument that process spawn overhead is irrelevant for the defined intervals is sound. The clear distinction and purpose of `init` and `health` commands are well-defined.

### 6. Crash Recovery
**Opinion:** **Agree.** The crash recovery mechanism is simple, elegant, and effective. Leveraging the SQLite database and the `running` status to identify and re-process (or dead-letter) orphaned jobs is a robust solution that avoids complex distributed transaction semantics. Its reliance on the exclusive advisory lock (Decision 12) is appropriate.

### 7. Protocol Specification (v1)
**Opinion:** **Agree.** A clear, versioned, and standardized protocol is essential for long-term maintainability and interoperability between the core and plugins. The design of the request and response envelopes covers all necessary communication details. Explicitly defining framing and mandating `EX_CONFIG` for protocol mismatches are excellent practices that enhance robustness. The manifest additions (especially `protocol` and `entrypoint`) are vital for reliable plugin discovery and execution.

### 8. Event Envelope
**Opinion:** **Agree.** A standardized event envelope simplifies routing logic and ensures consistent data flow between plugins. The chosen fields (`type`, `payload`, `dedupe_key`) are appropriate for core routing capabilities, with the core handling metadata injection (`source`, `timestamp`).

### 9. Plugin State Model
**Opinion:** **Strongly Agree.** The clear distinction between static `config` and dynamic `state` is critical and well-enforced. Storing `state` as a single JSON blob per plugin simplifies the core's interaction with plugin state, and the shallow merge for `state_updates` is pragmatic. The 1MB size limit is a good guardrail that forces plugins to remain lean and discourages inappropriate use of the state store for large data.

### 10. OAuth: Plugin-Owned
**Opinion:** **Strongly Agree.** This is another excellent decision that keeps the core generic and avoids an immense amount of complexity. OAuth flows are indeed provider-specific and constantly evolving; embedding this logic into the core would make it brittle and difficult to maintain. Delegating this to plugins, with the suggestion of shared helper libraries, is the optimal approach.

### 11. Webhook Security
**Opinion:** **Agree.** Mandating HMAC-SHA256 signature verification is fundamental for secure webhook reception. The proposed configuration and rejection behaviors (e.g., `403` without details, `413` for oversized bodies) are correct security practices. The pragmatic decision to defer replay protection and rate limiting (given the localhost binding and reliance on proxies) is appropriate for V1.

### 12. Multi-Instance Lock
**Opinion:** **Agree.** A PID file with `flock` is a simple, effective, and battle-tested mechanism for ensuring single-instance operation. Its reliance on kernel semantics for release on crash is robust and avoids potential "zombie lock file" issues.

### 13. Config Reload
**Opinion:** **Agree.** Using `SIGHUP` for graceful config reloads is standard Unix practice. The detailed handling of in-flight jobs, scheduler/router updates, and the lifecycle management of added/removed/disabled plugins is well-considered, ensuring a smooth transition without interrupting critical work.

### 14. Dispatch: Serial, Single Lane
**Opinion:** **Strongly Agree.** For the stated scale ("personal server, maybe 50 jobs/day"), introducing concurrency and priority lanes would be a significant over-engineering effort that adds substantial complexity (e.g., race conditions, deadlocks, resource contention) for minimal, if any, benefit. The current serial, FIFO approach is robust, predictable, and entirely "good enough." The reasoning about timeouts mitigating potential latency issues is sound.

### 15. Routing Semantics
**Opinion:** **Agree.** Fan-out routing provides excellent flexibility, allowing multiple plugins to react to a single event. The "all matches fire" approach is intuitive. The simplicity of exact string matching for event types and pushing conditional filtering into plugins (where process spawn overhead is deemed acceptable) keeps the router lean, fast, and focused on its core responsibility. Avoiding a query language in the router is a good design choice.

### 16. Plugin Trust & Execution
**Opinion:** **Agree.** These guidelines are essential for maintaining security, integrity, and predictable execution of plugins. Restrictions on location, explicit entrypoints, executability checks, and refusing insecure directory permissions are all sound practices. The recommendation for systemd `User=` for isolation is a good operational suggestion.

### 17. Jitter Behavior
**Opinion:** **Agree.** Computing jitter once per scheduled run, rather than re-randomizing per tick, correctly addresses the "schedule wander" problem. Snapping to `preferred_window` for hard constraints is also a sensible detail.

### 18. Logging & stderr
**Opinion:** **Agree.** The clear separation and structure for core logs and captured plugin `stderr` are crucial for effective debugging and operational monitoring. Reserving plugin `stdout` exclusively for protocol responses is a strong enforcement of the communication contract. The reminder about not logging secrets in `stderr` is important.

### 19. Job Log Retention
**Opinion:** **Agree.** Time-based retention is simple, predictable, and perfectly adequate for the expected job volume. Making it configurable and acknowledging its minimal impact on SQLite performance are appropriate.

### 20. Core Health Endpoint
**Opinion:** **Agree.** A simple, unauthenticated `/healthz` endpoint on localhost is an essential operational feature. It provides quick visibility into the core's status and is invaluable for monitoring systems (e.g., systemd watchdogs, basic scripts).

---

## Feedback Requested — Specific Challenges

### 1. Is at-least-once the right delivery guarantee, or should specific plugins get at-most-once semantics?
**Opinion:** **At-least-once is definitively the right default.** For this system, the complexity required to guarantee at-most-once delivery by the *core* would introduce significant overhead and fragility, contradicting the core's goal of simplicity. If an extremely sensitive plugin absolutely requires at-most-once semantics (e.g., for financial transactions where even a single duplicate is catastrophic), the responsibility for achieving this must lie entirely *within that specific plugin*. Such a plugin would need to implement its own highly robust, application-specific idempotency and potentially transaction management to ensure it only acts once, even if it receives multiple invocation attempts from the core. This is an advanced use case that should not burden the general-purpose core. The current `dedupe_key` provides a pragmatic level of idempotence for many common scenarios.

### 2. Is the circuit breaker (5 consecutive failures, 30m reset) too aggressive or too lenient?
**Opinion:** The circuit breaker settings (5 consecutive failures, 30-minute reset) are **well-balanced** for a personal integration gateway.
*   **5 consecutive failures:** This threshold is sensitive enough to quickly detect a persistently misbehaving or broken plugin (e.g., external API credential expiry, service outage) without tripping on transient network hiccups. It prevents continuous resource wastage.
*   **30-minute reset:** This duration provides a reasonable cool-down period. It allows sufficient time for transient external issues (e.g., rate limits, external service outages) to resolve or for an operator to notice and intervene. For a personal server, a 30-minute delay before automatically retrying a potentially broken plugin is a very acceptable trade-off between resilience and responsiveness. More complex circuit breaker patterns (e.g., half-open states, sliding window error rates) would add unnecessary complexity for this project's scope.

### 3. Is spawn-per-command viable for plugins that need to maintain persistent connections (e.g., WebSocket listeners)?
**Opinion:** **No, spawn-per-command is explicitly NOT viable for plugins that need to maintain persistent, long-lived connections** like WebSocket listeners, persistent database connections, or long-polling HTTP clients.
*   The spawn-per-command model dictates that a plugin process starts, performs a single command (poll, handle, health, init), and then exits. This fundamental design choice is in direct conflict with the requirement of a persistent connection, which demands a long-running process that maintains state and reacts to asynchronous events over an extended period.
*   **Stance:** This RFC should explicitly state that plugins requiring persistent, long-lived connections are **out of scope** for the current architecture. If such functionality were ever required, it would necessitate a completely different (and far more complex) plugin execution model within the core (e.g., a managed worker process pool with its own communication protocols and lifecycle management), which would contradict the simplicity goals of this RFC.

### 4. Should the core eventually support a "streaming" plugin mode for long-lived consumers, or is that out of scope forever?
**Opinion:** It should remain **out of scope forever** for *this specific core*.
*   Introducing a "streaming" or "long-lived consumer" mode would fundamentally transform the `senechal-gw` core into a distributed stream processing framework, which is a significantly more complex architectural undertaking. This would entail managing long-running processes, complex inter-process communication, backpressure handling, sophisticated error recovery for persistent connections, and more.
*   The current architecture's strength lies in its simplicity, robustness, and stateless (from the core's perspective) interaction with ephemeral plugin processes. Attempting to add a streaming mode would undermine these core tenets.
*   If streaming or long-lived consumption capabilities become a critical need, the recommended approach would be to deploy a dedicated, external streaming system (e.g., a message queue, a separate service) that `senechal-gw`'s existing spawn-per-command plugins can interact with (e.g., producing events to a queue, or having a dedicated `handle` command pull from a queue), thereby maintaining `senechal-gw`'s architectural boundaries and focused scope.
