---
id: 92
status: done
priority: High
blocked_by: []
tags: [architecture, pipelines, interactive, dag, critical, implementation-ready]
---

# RFC-92: Pipeline Execution Modes (Explicit Opt-In)

## Status Update (2026-02-13)

**Implementation Complete**. RFC-92 successfully implemented and verified.
- **Async by default** (preserves current event-driven architecture)
- **Sync opt-in** (explicit per-pipeline via `execution_mode: synchronous`)
- **Resource Guarded** (semaphore-based concurrency control)
- **No breaking changes** (existing pipelines unchanged)

## Executive Summary

**Problem**: ductile's async-only execution blocks interactive use cases (Discord bots, web UIs, CLI tools). Users get "queued" responses but never receive actual results.

**Solution**: Add optional `execution_mode: synchronous` to pipeline config. Pipelines explicitly declare whether they should wait for completion before returning results.

**Key Principle**: **Async by default** (preserve event-driven architecture), **sync by opt-in** (enable interactive use cases).

## Design Philosophy

### Preserve Event-Driven Architecture

**Current architecture (keep this)**:
- Events trigger pipelines
- Steps emit events (`on_events`)
- Dispatcher queues jobs asynchronously
- Parent jobs don't block on children

**What we added**:
- Optional synchronous wait at API boundary (trigger endpoints) using a "Guarded Bridge"
- Internal event flow remains unchanged
- Dispatcher still processes jobs asynchronously
- API handler waits for completion before responding

### When to Use Each Mode

| Mode | Use Cases | Example Pipelines |
|------|-----------|-------------------|
| **async** (default) | Batch processing, webhooks, long-running jobs, fire-and-forget | File uploads, scheduled jobs, background processing |
| **synchronous** (opt-in) | Interactive commands, chat bots, CLI tools, web APIs | Discord commands, CLI queries, API endpoints that return results |

## Proposed Schema

### Pipeline Configuration (GitHub-like Notation)

```yaml
# pipelines/youtube-summary.yaml
pipelines:
  - name: youtube-to-discord-summary
    on: discord.command.youtube    # Triggered by Discord slash command or webhook
    execution_mode: synchronous     # The API call blocks until the pipeline finishes
    timeout: 3m                    # Extended timeout for downloads/LLM processing
    steps:
      - id: download
        uses: youtube.download     # Downloads & extracts audio/text
        
      - id: summarize
        uses: fabric.summarize     # Processes transcript through LLM
        
      - id: archive
        uses: file_handler.save    # Persists the summary to the workspace
        
      - id: notify
        uses: discord.respond      # Sends the summary back to the Discord user
```

### Configuration Fields (Updated)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `execution_mode` | `async` \| `synchronous` | `async` | How trigger endpoint behaves |
| `timeout` | duration | `30s` | Max wait time for synchronous pipelines |
| `steps[].id` | string | (optional) | Unique identifier for the step |
| `steps[].uses` | string | (required) | Plugin command to execute |

## How It Works

### Key Insight: Wait at API Boundary, Not Internal Execution

**Internal execution remains event-driven**:
- Dispatcher still queues jobs asynchronously
- Events still trigger next steps
- No blocking in dispatcher/runner

**Wait happens only at HTTP layer**:
- API handler uses a "Guarded Bridge" (completion channels in Dispatcher)
- Returns results when complete
- Or returns timeout (202 Accepted) if exceeded

This preserves event-driven architecture while enabling sync API responses.

## Implementation Details

### Core Implementation

- [x] `internal/config/types.go` - Add ExecutionMode, Timeout fields for YAML
- [x] `internal/router/dsl/types.go` - Add ExecutionMode, Timeout fields for runtime
- [x] `internal/config/loader.go` - Parse execution_mode, timeout
- [x] `internal/dispatch/dispatcher.go` - Add WaitForJobTree, completion notifications
- [x] `internal/api/handlers.go` - Check execution mode, wait for sync pipelines
- [x] `internal/api/types.go` - Add SyncResponse, TimeoutResponse types
- [x] `internal/api/server.go` - Add semaphore for rate limiting, adjust WriteTimeout

### Configuration

- [x] `config.yaml` - Add api.max_sync_timeout, api.max_concurrent_sync

### Documentation

- [x] `docs/PIPELINES.md` - Document execution_mode field
- [x] `docs/API_REFERENCE.md` - Document new response formats

### Testing

- [x] `internal/dispatch/dispatcher_test.go` - Test WaitForJobTree
- [x] `internal/api/sync_test.go` - Test sync/async modes and semaphore
- [x] Fixed all project test regressions due to protocol v2 mismatch.

## Success Criteria

- [x] Discord bot receives actual fabric output (not "queued")
- [x] Async pipelines unchanged (backward compatible)
- [x] Sync pipelines return results within timeout
- [x] HTTP resource limits enforced (no starvation)
- [x] Event-driven architecture preserved
- [x] Documentation complete with examples

## Narrative

- 2026-02-13: Created to solve interactive use case problem (RFC-91)
- 2026-02-13: Refactor-93 put on hold due to critique (breaking changes, HTTP blocking)
- 2026-02-13: RFC-92 updated with detailed implementation, async default, explicit opt-in approach. Addresses all critique concerns while enabling interactive use cases.
- 2026-02-13: Technical Critique: RFC-92 is the superior approach because it preserves the event-driven core. Tying the "wait" to the API boundary rather than the internal dispatcher loop prevents architectural regression. Recommendation: 1) Ensure Phase 5 (Semaphores) is prioritized to prevent HTTP pool exhaustion. 2) Define a strict aggregation schema for the `SyncResponse` to handle `split` and parallel branch results consistently. 3) Treat sync mode as a "guarded bridge" between synchronous clients and the async engine. (by @assistant)
- 2026-02-13: Notation & Technical Requirements Update: Defined "GitHub-like" notation for pipelines (e.g., YouTube summary). Refined implementation plan to include "Guarded Bridge" dispatcher logic and "DSL-to-Event" routing automation. (by @assistant)
- 2026-02-13: Technical Finalization: Corrected implementation targets to match `internal/api` and `internal/router`. Addressed critical gaps in trigger mapping (`plugin.command`), server timeouts, and job terminal states. Narrowed scope by deferring step-level DSL keywords (`async/parallel`) to focus on the core synchronous API response goal. Ready for implementation. (by @assistant)
- 2026-02-13: Implementation Complete: All phases of RFC-92 implemented and verified. Gateway now supports synchronous pipeline execution with resource guarding and robust error handling. (by @assistant)
- 2026-02-13: Testing Feedback: End-to-end testing of synchronous triggers (including Discord bot simulation) was successful. The "Guarded Bridge" correctly aggregates multi-hop results and handles timeouts gracefully. (by @assistant)
