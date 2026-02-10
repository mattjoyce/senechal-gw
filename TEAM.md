# Senechal Gateway Development Team

## Team Roster

**Team Lead:** Claude (coordination, architecture decisions)

**Agent 1 (Claude):** Configuration & Plugin System
**Agent 2 (Codex):** State & Queue Management
**Agent 3 (Gemini):** Observability & Orchestration

## Branch Naming Convention

**IMPORTANT:** All branches MUST be prefixed with agent codename:
- Agent 1 (Claude): `claude/*`
- Agent 2 (Codex): `codex/*`
- Agent 3 (Gemini): `gemini/*`

Examples: `claude/dispatch`, `codex/metrics`, `gemini/scheduler-integration`

## Phase Status

### ✅ Phase 0: Complete
- Crash recovery decision (Option B)
- Go project skeleton

### ✅ Phase 1: Complete (Merged to main)

**Agent 1 (Claude):**
- ✅ ID 13: Protocol v1 Codec
- ✅ ID 10: Config Loader + Env Interpolation
- ✅ ID 12: Plugin Discovery + Manifest Validation
- Branch: `claude/config-plugin` (merged)

**Agent 2 (Codex):**
- ✅ ID 11: SQLite Schema Bootstrap
- ✅ ID 14: SQLite Work Queue
- ✅ ID 17: Plugin State Store
- ✅ ID 18: PID Lock
- Branch: `codex/state-queue` (merged)

**Agent 3 (Gemini):**
- ✅ ID 19: Structured JSON Logging
- Branch: `gemini/logging` (merged)

### ✅ Phase 2: Integration (Complete)

**Agent 3 (Gemini):** Scheduler & Orchestration
- Branch: `gemini/scheduler` ✅ Merged (PR #6)
- ✅ ID 15: Scheduler Tick Loop + Fuzzy Intervals
- ✅ ID 25: Crash Recovery Implementation
- Deliverable: Scheduler enqueues jobs, handles orphan recovery ✅

**Agent 1 (Claude):** Dispatch Loop
- Branch: `claude/dispatch` ✅ Merged (PR #4)
- ✅ ID 16: Dispatch Loop (spawn plugin, protocol I/O, timeouts)
- Deliverable: Can execute plugins via subprocess ✅

**Agent 2 (Codex):** Integration & Validation
- Branch: `codex/integration` ✅ Merged (PR #5)
- ✅ ID 20: Echo Plugin E2E Runbook (validation)
- Deliverable: E2E tests passing ✅

**All Agents:** Sprint Epic
- 🔄 ID 8: MVP Core Loop (Status: DOING - final wiring needed)

### ✅ Phase 3: Final Integration (Complete)

**Agent 1 (Claude):** Main.go Wiring
- Branch: `claude/main-cli` ✅ (PR pending)
- ✅ ID 26: Wire MVP Components in main.go
  - Complete runStart() function with all component initialization
  - Signal handling (SIGINT/SIGTERM) and graceful shutdown
  - PID lock, database, queue, state, plugins, scheduler, dispatcher
- Deliverable: `senechal-gw start` runs complete MVP loop ✅

**Sprint 1 MVP COMPLETE!** All tests passing, binary functional, echo plugin executes successfully.

## Agent Capability Assessment

Based on Phase 1 & 2 execution:
- **Agent 1 (Claude):** Complex multi-component work, strong architecture understanding
- **Agent 2 (Codex):** Solid implementation, good with focused tasks and testing
- **Agent 3 (Gemini):** Produces high-quality code, best with well-defined single-component tasks

---

## ✅ Sprint 2: API Triggers (Complete)

**Goal:** Enable LLM to curl-trigger plugins and retrieve results

### Agent 1 (Claude): HTTP Server + API Endpoints
- **Branch:** `claude/api-server` ✅ Merged (PR #10)
- **Card:** #28 - HTTP Server + API Endpoints
- ✅ Chi router with graceful shutdown
- ✅ POST /trigger/{plugin}/{command} - enqueue job, return job_id
- ✅ GET /job/{job_id} - return status and results
- ✅ Auth middleware integration
- ✅ Integration and E2E tests
- Deliverable: HTTP API server functional ✅

### Agent 2 (Codex): Job Storage + Auth
- **Branch:** `codex/job-storage-auth` ✅ Merged (PR #8)
- **Card:** #29 - Job Storage Enhancement + Auth Middleware
- ✅ Enhanced job_log table with result column
- ✅ Store plugin response payload on completion
- ✅ Implemented GetJobByID() for result retrieval
- ✅ API key validation functions
- ✅ Unit tests for storage and auth
- Deliverable: Job storage with auth helpers ✅

### Agent 3 (Gemini): User Guide Documentation
- **Branch:** `gemini/user-guide` ✅ Merged (PR #9)
- **Card:** #30 - User Guide Documentation
- ✅ Comprehensive USER_GUIDE.md (2000+ words)
- ✅ Setup, configuration, usage instructions
- ✅ Plugin development guide
- Deliverable: Complete user documentation ✅

**Sprint 2 COMPLETE!** LLM can now curl-trigger plugins and retrieve results.

---

## 🔄 Sprint 3: Webhooks + Security + Observability (Current)

**Goal:** Secure 3rd party webhook integrations with token-based auth and real-time observability

**Foundation:** Multi-file config enables LLM-safe editing. Token scopes prevent over-permissioned access. SSE events enable real-time debugging.

### Agent 1 (Claude): Multi-File Config + Webhooks
- **Branch:** `claude/sprint3-config-webhooks`
- **Cards:** #39, #42
- **Deliverable:** Multi-file configuration system + webhook endpoints
- **Work:**
  - #39: Multi-file config system (`~/.config/senechal-gw/` directory structure)
  - BLAKE3 hash verification on scope files
  - Cross-file reference validation
  - #42: Webhook listener with HMAC-SHA256 verification (after #39)
  - POST /webhook/{path} endpoints from webhooks.yaml
  - Body size limits, job enqueueing
- **Dependencies:** None (foundation work)
- **Complexity:** Medium-High, foundational infrastructure

### Agent 2 (Codex): Metadata + Auth + Observability
- **Branch:** `codex/sprint3-metadata-auth-obs`
- **Cards:** #36, #35, #33, #43
- **Deliverable:** Manifest metadata + token auth + observability endpoints
- **Work:**
  - #36: Manifest command type metadata (read vs write)
  - #35: Token scopes with manifest-driven permissions (after #36, needs #39)
  - #33: SSE /events endpoint for real-time debugging
  - #43: /healthz endpoint for monitoring
- **Dependencies:** #36 blocks #35, #35 needs #39 from Agent 1
- **Complexity:** Medium, multiple discrete tasks

### Merge Order
1. **Agent 2** (codex/sprint3-metadata-scopes) - #36 first, then rest
2. **Agent 1** (claude/sprint3-config-webhooks) - #39 first, then #42
3. **Agent 3** (Gemini) - Documentation after Sprint 3 dev work merged

**Note:** Card #29 status updated to `done` (was showing `todo` but PR #8 merged)

---

## Future Sprints

### Sprint 4: Reliability Controls
- Circuit breaker (auto-pause failing plugins)
- Retry with exponential backoff
- Deduplication enforcement
- Event routing (plugin chaining)

### Post-Sprint 4: RFC-003 Evaluation
Card #27 - Assess evolution toward "Agentic Loop Runtime" or mature as automation tool.
Decision deferred until real usage data available.

See `kanban/rfc-003-evaluation.md` and `RFC-003-Agentic-Loop-Runtime.md` for details.
