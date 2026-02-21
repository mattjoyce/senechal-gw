---
id: 102
status: done
priority: high
tags: [api, pipelines, triggers, design]
---

# Improvement 102: Separate Plugin and Pipeline Trigger Endpoints

**Type:** improvement

## Problem

The current `/trigger/{plugin}/{command}` endpoint is ambiguous and confusing:

1. **Unclear Intent**: Calling `/trigger/jina-reader/handle` may run:
   - Just the jina-reader plugin (if no pipeline matches)
   - The entire url-to-fabric pipeline (if pipeline has `on: jina-reader.handle`)

2. **No Direct Plugin Execution**: Cannot execute a plugin directly without risk of triggering a pipeline that listens to that event.

3. **Hidden Pipeline Activation**: Users calling a plugin endpoint don't expect a multi-step pipeline to start automatically.

4. **Design Confusion**: The endpoint name suggests "trigger this plugin" but actually means "emit this event which may trigger pipelines".

## User Expectation

**When I call a plugin, run that plugin.**
**When I call a pipeline, run that pipeline.**

## Current Behavior

```bash
# User thinks: "Run jina-reader to scrape this URL"
POST /trigger/jina-reader/handle {"payload": {"url": "..."}}

# What actually happens: Runs url-to-fabric pipeline (jina-reader → fabric)
# because pipeline listens to jina-reader.handle event
```

## Desired Behavior

### Option A: Separate Endpoints

```bash
# Direct plugin execution (no pipeline routing)
POST /plugin/{plugin}/{command}
→ Runs only the specified plugin

# Explicit pipeline execution
POST /pipeline/{pipeline_name}
→ Runs the named pipeline

# Legacy endpoint (deprecated but kept for compatibility)
POST /trigger/{plugin}/{command}
→ Keep current behavior with deprecation warning
```

### Option B: Explicit Triggering

```bash
# Plugin execution (default, no pipeline routing)
POST /trigger/jina-reader/handle
→ Runs jina-reader only

# Explicit pipeline trigger
POST /trigger/pipeline/url-to-fabric
→ Runs url-to-fabric pipeline
```

## Goals

Support all execution contexts:
- ✅ **Scheduled**: Cron-based polling via scheduler
- ✅ **Manual**: Direct API calls for testing/debugging
- ✅ **Pipeline**: As a step within a larger workflow
- ✅ **Event-driven**: In response to emitted events

**All contexts should support both:**
- Direct plugin execution (atomic operation)
- Pipeline execution (orchestrated workflow)

## Proposed Solution

1. **New Endpoints**:
   - `POST /plugin/{plugin}/{command}` - Direct plugin execution, no pipeline routing
   - `POST /pipeline/{pipeline}` - Explicit pipeline execution

2. **Deprecate Ambiguous Behavior**:
   - Keep `/trigger/{plugin}/{command}` for backward compatibility
   - Add deprecation warning header
   - Document migration path

3. **Pipeline Event Model**:
   - Pipelines should use domain events, not plugin command events
   - Example: `on: url.submitted` instead of `on: jina-reader.handle`
   - Plugins emit completion events: `jina_reader.scraped`, `fabric.completed`
   - Pipelines chain via event routing, not command hijacking

## Implementation Steps

1. Add `/plugin/{plugin}/{command}` endpoint (bypasses router)
2. Add `/pipeline/{pipeline}` endpoint
3. Update router to distinguish plugin vs pipeline triggers
4. Add deprecation headers to `/trigger" endpoint
5. Update documentation with examples
6. Migrate example pipelines to use domain events
7. Update tests

## Related

- Current behavior confuses API consumers
- Makes debugging difficult (unclear what will execute)
- Violates principle of least surprise

## Acceptance Criteria

- [x] Can execute plugin directly without triggering pipelines
- [x] Can execute pipeline explicitly by name
- [x] Scheduler works with both endpoints
- [x] Event routing still works for pipeline composition
- [x] Backward compatibility maintained
- [x] Clear error messages when wrong endpoint used
- [x] Documentation updated with examples

## Notes

This is a fundamental UX improvement. The current design treats everything as events, which is powerful but confusing. Users need explicit control over whether they're running a plugin or a pipeline.

## Critique Notes (2026-02-15)

- The card is still planning-only (`status: backlog`) and none of the acceptance criteria are implemented yet.
- Current API behavior remains ambiguous: `/trigger/{plugin}/{command}` still resolves pipeline triggers, creates event context, and may execute a multi-step flow.
- The card contains two competing options (A and B) without selecting one, which blocks clear implementation scope and test design.
- The implementation step "Update router to distinguish plugin vs pipeline triggers" is likely misplaced; endpoint semantics should be handled in the API layer, while router remains event routing logic.
- Acceptance criterion "Scheduler works with both endpoints" is not directly applicable because scheduler enqueues jobs internally, not through HTTP endpoints.
- Deprecation needs explicit policy details (header name, migration guidance, and sunset timeline).

## Narrative

- 2026-02-15: Created to address trigger ambiguity. (by @mattjoyce)
- 2026-02-15: **Critique by @gemini**:
    - Strong alignment with RFC-004 "Safety Boundary" goals.
    - **Recommendation**: Proceed with **Option A** (`/plugin/...` and `/pipeline/...`) as it is more idiomatic and prevents deep nesting.
    - **Synchronous requirement**: The `/pipeline` endpoint MUST support blocked `synchronous` execution mode so LLM operators can await full workflow results.
    - **Discovery**: The `ductile skill` command should be updated to partition capabilities by these new endpoints.
    - **Conclusion**: This moves Ductile from a passive event bus toward an active orchestration gateway. Suggested move to `doing`.
- 2026-02-15: Added an implementation critique to clarify why #102 remains backlog and to highlight required decisions before coding (endpoint model selection, API-layer responsibility, and precise acceptance criteria). (by @codex)
- 2026-02-15: **Testing session validation** (by @claude):
    - Confirmed real-world UX issue: User called `/trigger/jina-reader/handle` expecting scrape-only operation, but `url-to-fabric` pipeline executed (scrape → fabric summarize).
    - Current API handler (`internal/api/handlers.go:96-113`) always checks `router.GetPipelineByTrigger()` before job enqueue, making direct plugin execution impossible.
    - **Implementation path confirmed**: Option A (`/plugin/...` and `/pipeline/...`) requires new API handlers that bypass router for `/plugin/*` but invoke router for `/pipeline/*`.
    - **Backward compatibility note**: Existing `/trigger` endpoint behavior preserved means pipelines using `on: {plugin}.{command}` continue working, but documentation should migrate to domain events.
    - **Skill manifest impact**: `ductile skill` output should list both atomic plugin skills (via `/plugin`) and orchestrated pipeline skills (via `/pipeline`) separately for clarity.
    - **Move to doing supported**: This addresses a fundamental usability gap discovered through live testing.
- 2026-02-22: Verified implementation in `internal/api/server.go` and `internal/api/handlers.go`. `/plugin` and `/pipeline` endpoints functional, `/trigger` deprecated. (by @assistant)
