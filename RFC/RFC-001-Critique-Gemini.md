# RFC-001-Critique-Gemini: Senechal Gateway Review

**Reviewer:** Gemini CLI
**Date:** 2026-02-08
**RFC Status:** Draft
**Author:** Matt Joyce

---

## Overall Impression

This is a well-structured and thoughtful RFC, clearly addressing a common pain point with existing monoliths and overly complex integration platforms. The proposal for a lightweight, modular, and YAML-configured gateway with a Go core and polyglot subprocess plugins is compelling. The architectural diagram is clear, and the key decisions table effectively summarizes the rationale behind the choices. The phased implementation plan is also a good touch, showing a clear path forward.

## Specific Feedback and Critique

### Problem

*   **Strength:** The problem statement is clear, concise, and directly identifies the current limitations of the FastAPI monolith and the overhead of existing integration servers. This sets a strong foundation for the proposed solution.

### Proposal

*   **Strength:** The proposal is equally clear. The emphasis on "lightweight, YAML-configured, modular" resonates well with the problem statement. The "understand in an afternoon, extensible enough to grow" promise is a good aspirational goal.

### Architecture Overview

*   **Strength:** The diagram is excellent — easy to follow and clearly illustrates the data flow and component interactions. The explicit mention of "~1 process" for the Go core highlights the lightweight nature.
*   **Minor Suggestion:** Consider adding a small legend for the arrows (e.g., "data flow," "control flow") if there are distinct types, though it's fairly intuitive as is.

### Key Decisions

*   **Strength:** This table is incredibly useful for understanding the core tenets of the design. The rationales are sound for a "personal service."
*   **Critique/Consideration (related to future scalability):**
    *   **"Execution: Serial dispatch"**: While simple and predictable for personal use, this will be a significant bottleneck if throughput ever becomes a concern. For a personal gateway, it's likely fine, but it's important to acknowledge this limitation explicitly. If a plugin takes a long time, the entire queue stalls. Perhaps a "max parallel jobs" configuration could be a future consideration without fully embracing complex concurrency.

### Core Components

#### 1. Work Queue

*   **Strength:** "Everything that produces work submits to the queue" is a powerful and elegant unifying concept. The job structure looks comprehensive.
*   **Feedback Requested (1): Is the work queue as central abstraction the right model?** Yes, absolutely. For an event-driven system focused on decoupling and reliability, a work queue is foundational and an excellent choice. It simplifies coordination, provides backpressure, and enables persistence for crash recovery.
*   **Minor Suggestion:** The `priority` field is listed but not explicitly discussed. How will it be used? Is it an integer (higher = higher priority)? How does it interact with serial execution (does it preempt the current job, or just order the next available one)? This is a minor detail but worth clarifying.

#### 2. Scheduler (Heartbeat + Fuzzy Intervals)

*   **Strength:** "Human-friendly intervals with jitter" is a fantastic design choice, avoiding the complexity of cron syntax while offering practical flexibility. The `preferred_window` is also a great addition for personal energy consumption or API rate limit considerations.
*   **Minor Suggestion:** Clarify how `jitter` interacts with `preferred_window`. Does the jitter apply *within* the preferred window, or can it push the schedule *outside* if the window is too narrow? (e.g., "daily with 2h jitter" and window "06:00-07:00" – could it run at 07:30?)

#### 3. Plugin Protocol (JSON over stdin/stdout)

*   **Strength:** "Language-agnostic, fault-isolated, drop-in plugins" are strong arguments for this approach. The specified commands and responses are clear and cover the necessary interactions. The `manifest.yaml` is also well-defined.
*   **Feedback Requested (2): Is subprocess (JSON over stdin/stdout) the right plugin boundary?** Yes, for the stated goals of polyglot support and fault isolation, this is an excellent choice. It's a proven pattern (e.g., HashiCorp's plugin system). It introduces some overhead per invocation compared to in-process plugins, but the benefits of language agnosticism and isolation generally outweigh this for many use cases, especially for a personal gateway where raw throughput isn't the primary driver.
*   **Minor Suggestion:** A `timeout` field in the manifest or config for each plugin could be useful to control how long the core waits for a plugin response before considering it failed.

#### 4. Config-Declared Routing

*   **Strength:** This decision reinforces the "plugins stay dumb, core controls flow" philosophy, which is critical for manageability and preventing plugins from becoming too tightly coupled.
*   **Minor Suggestion:** Consider if there's a need for conditional routing in the future (e.g., "if `event_type` is `alert` AND `severity` is `critical`, then `to: pagerduty`"). This is a future enhancement, but good to keep in mind.

#### 5. Configuration (config.yaml)

*   **Strength:** The YAML structure is clean and intuitive. Environment variable interpolation for secrets is a must-have and well-implemented. The examples are clear.

#### 6. State Store (SQLite)

*   **Strength:** SQLite for persistence is an excellent "zero-ops" choice for a personal gateway. The separation of `plugin_state` and `job_queue` is sensible.
*   **Clarification:** "Plugins receive their state slice with each invocation and return state updates." This implies a clear contract, which is good.

#### 7. CLI

*   **Strength:** A robust CLI is essential for a tool like this, and the proposed commands cover all critical operational aspects.

#### 8. Service Deployment

*   **Strength:** Providing a Systemd unit example is very helpful for production readiness.

### Project Layout

*   **Strength:** A standard Go project layout, which is easy to navigate and understand.

### Implementation Phases

*   **Strength:** A logical and progressive breakdown of work. Starting with skeleton and core loop, then adding more advanced features, is a solid approach.

### Open Questions

*   **Plugin timeout handling — what happens when a plugin hangs?** This is critical. I'd lean towards configurable timeouts per plugin in the `config.yaml` or `manifest.yaml`. Upon timeout, the core should kill the subprocess and mark the job as failed, potentially with retry logic.
*   **Plugin stderr — capture as logs or discard?** Capture as logs. `stderr` often contains crucial debugging information for plugin failures. It should be directed to the gateway's logging system, perhaps with a specific log level (e.g., `WARN` or `ERROR`) and tagged with the plugin's name.
*   **Config reload semantics — what happens to in-flight jobs when config changes?** This needs careful thought.
    *   Simplest: Only apply new config to *new* jobs. In-flight jobs continue with the old config. This is safest but leads to a transitional state.
    *   More complex: Attempt to gracefully stop/restart plugins if their config changes (very tricky with subprocesses).
    *   Recommendation: For a personal gateway, the simplest approach might be acceptable: `reload` applies changes to subsequent scheduled/triggered jobs. A full `restart` might be required for changes affecting actively running jobs or plugin definitions. The `reload` command could simply re-read the configuration and update the scheduler and router with the new rules for *future* operations.
*   **Multi-instance safety — should SQLite locking prevent two instances running simultaneously?** Yes, absolutely. Running two instances against the same SQLite database would lead to corruption and race conditions. A file-based lock (e.g., `.lock` file) at startup or using SQLite's built-in file locking mechanisms (though less robust for crash recovery than advisory locks) should be implemented.
*   **Plugin versioning — how to handle manifest version mismatches?** The `version` in `manifest.yaml` is a good start. The core should probably refuse to load plugins that declare incompatible versions (e.g., if the core expects protocol `v1.0` and a plugin manifest declares `v0.5` or `v2.0`). This could be handled via a `protocol_version` field in the manifest, distinct from the plugin's own `version`.
*   **OAuth token refresh — who manages token lifecycle, the core or the plugin?** For simplicity and to keep plugins "dumb," the core should manage tokens *if* it understands the OAuth flow. However, this often requires HTTP redirects and complex state, pushing back towards a more heavyweight core. A pragmatic approach for V1 might be:
    *   **Plugin handles:** The plugin is responsible for its token lifecycle, storing refresh tokens in `plugin_state` and performing refreshes as needed. The core just passes the current state. This keeps the core simpler.
    *   **Core provides utility:** The core could provide a simplified "OAuth helper" command via the protocol that a plugin can invoke (e.g., `{"command": "oauth_refresh", "plugin_name": "...", "client_id": "..."}`), where the core then handles the HTTP interactions. This adds complexity to the core but simplifies plugins.
    *   **Recommendation:** Start with plugins handling their own token refreshes, storing necessary data in `plugin_state`. This aligns with the "plugins stay dumb" philosophy for interactions, but allows them to manage their external dependencies.

### Feedback Requested Summary

1.  **Is the work queue as central abstraction the right model?** Yes, it's an excellent choice for this architecture, providing robustness and clear separation of concerns.
2.  **Is subprocess (JSON over stdin/stdout) the right plugin boundary?** Yes, it aligns perfectly with the goals of language agnosticism and fault isolation, suitable for a personal integration gateway.
3.  **Is Go the right choice for the core, given polyglot plugins?** Yes, Go's strengths (single binary, easy deployment, natural subprocess spawning, strong concurrency primitives if needed later) make it an ideal choice for this kind of orchestrator.
4.  **What's missing from this design?**
    *   More explicit error handling and retry mechanisms for failed jobs/plugins.
    *   Graceful shutdown procedures for the core and plugins.
    *   A clearer plan for OAuth token management (as discussed above).
    *   Monitoring/metrics (e.g., Prometheus exporter for queue depth, job success/failure rates). This is a V2 feature but good to consider.
5.  **What's over-engineered for a personal integration server?**
    *   **Nothing significantly over-engineered.** The design hits a good balance. The serial dispatch is a deliberate simplification, which is appropriate. The fuzzy scheduling and config-declared routing add valuable features without excessive complexity.

## Conclusion

This is a very strong RFC. The design is coherent, addresses the stated problems effectively, and makes pragmatic choices suitable for a "personal integration gateway." The clear identification of open questions demonstrates a thorough thought process. I'm excited to see this progress!
