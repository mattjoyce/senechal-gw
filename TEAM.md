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

## 🔄 Sprint 2: API Triggers (Current)

**Goal:** Enable LLM to curl-trigger plugins and retrieve results

### Agent 1 (Claude): HTTP Server + API Endpoints
- **Branch:** `claude/api-server`
- **Card:** #28 - HTTP Server + API Endpoints
- **Deliverable:** HTTP server with POST /trigger and GET /job endpoints
- **Work:**
  - Chi router with graceful shutdown
  - POST /trigger/{plugin}/{command} - enqueue job, return job_id
  - GET /job/{job_id} - return status and results
  - Auth middleware integration
  - Wire to main.go alongside scheduler/dispatcher
  - Integration and E2E tests
- **Dependencies:** Uses interface from Agent 2 (can develop with mocks in parallel)

### Agent 2 (Codex): Job Storage + Auth
- **Branch:** `codex/job-storage-auth`
- **Card:** #29 - Job Storage Enhancement + Auth Middleware
- **Deliverable:** Enhanced job storage with results + API key validation
- **Work:**
  - Add result column to job_log table
  - Store plugin response payload on completion
  - Implement GetJobByID() for result retrieval
  - API key validation functions
  - Update dispatcher to pass results to Complete()
  - Unit tests for storage and auth
- **Merge first:** Agent 1 depends on this interface

### Agent 3 (Gemini): User Guide Documentation
- **Branch:** `gemini/user-guide`
- **Card:** #30 - User Guide Documentation
- **Deliverable:** Comprehensive user guide at `docs/USER_GUIDE.md`
- **Work:**
  - Document MVP features (scheduler, plugins, state, crash recovery)
  - Step-by-step setup and usage instructions
  - Configuration reference with examples
  - Plugin development guide (Bash and Python examples)
  - Operations and troubleshooting sections
  - 2000-3000 words, well-structured Markdown
- **Independent:** No code dependencies, can merge anytime

### Merge Order
1. **Agent 2** (codex/job-storage-auth) - Foundation for Agent 1
2. **Agent 1** (claude/api-server) - Uses Agent 2's interface
3. **Agent 3** (gemini/user-guide) - Documentation, no conflicts

---

## Future Sprints

### Sprint 3: Webhooks + Event Routing
- External webhook triggers (GitHub, Slack)
- Internal plugin→plugin event routing
- HMAC validation, routing table

### Sprint 4: Reliability Controls
- Circuit breaker (auto-pause failing plugins)
- Retry with exponential backoff
- Deduplication enforcement
- /healthz endpoint

### Post-Sprint 4: RFC-003 Evaluation
Card #27 - Assess evolution toward "Agentic Loop Runtime" or mature as automation tool.
Decision deferred until real usage data available.

See `kanban/rfc-003-evaluation.md` and `RFC-003-Agentic-Loop-Runtime.md` for details.
