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

### 🔄 Phase 2: Integration (Current)

**Agent 3 (Gemini):** Scheduler & Orchestration
- Branch: `gemini/scheduler`
- 🔲 ID 15: Scheduler Tick Loop + Fuzzy Intervals
- 🔲 ID 25: Crash Recovery Implementation
- Deliverable: Scheduler enqueues jobs, handles orphan recovery

**Agent 1 (Claude):** Dispatch Loop
- Branch: `claude/dispatch`
- 🔲 ID 16: Dispatch Loop (spawn plugin, protocol I/O, timeouts)
- Deliverable: Can execute plugins via subprocess

**Agent 2 (Codex):** Integration Support
- Assists with dispatch integration
- 🔲 ID 20: Echo Plugin E2E Runbook (validation)

**All Agents:** Sprint Completion
- 🔲 ID 8: MVP Core Loop Integration Testing