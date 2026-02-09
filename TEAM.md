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
## Next Steps

### Immediate: Merge Phase 3 PR
- Review PR on `claude/main-cli` branch
- Merge to main
- Tag as v0.1.0-mvp

### Sprint 2-4: Core Features (Per SPEC.md)
- Sprint 2: Event Routing (plugin chaining)
- Sprint 3: Webhooks + /healthz
- Sprint 4: Reliability Controls (circuit breaker, retry, deduplication)

### Future: RFC-003 Evaluation (After Sprint 4)
Card #27 created for post-Sprint 4 reflection:
- Assess whether to evolve toward "Agentic Loop Runtime" vision
- Or mature as personal automation tool
- Decision deferred until we have real usage data

See `kanban/rfc-003-evaluation.md` and `RFC-003-Agentic-Loop-Runtime.md` for details.
