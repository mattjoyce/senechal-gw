# Multi-Agent Development Coordination

## Environment Setup

**Go Version:** 1.25.4 (installed at `/opt/homebrew/bin/go`)
**Minimum Required:** Go 1.21+ (per golang-pro skill and SPEC)
✅ Current version is compatible

## Commit Conventions

**Format:**
```
<component>: <imperative verb> <what>

<optional body explaining why>
```

**Component prefixes:**
- `config:` - internal/config package
- `state:` - internal/state package
- `queue:` - internal/queue package
- `scheduler:` - internal/scheduler package
- `dispatch:` - internal/dispatch package
- `plugin:` - internal/plugin package
- `webhook:` - internal/webhook package
- `router:` - internal/router package
- `cli:` - cmd/senechal-gw
- `protocol:` - protocol v1 types/codec
- `docs:` - documentation changes
- `test:` - test infrastructure
- `chore:` - build, deps, tooling

**Examples:**
```
config: implement YAML parser with env interpolation

queue: add SQLite-backed FIFO enqueue/dequeue

dispatch: enforce timeout with SIGTERM → SIGKILL sequence
```

**Rules:**
- NEVER attribute Claude or AI (per ~/.claude/CLAUDE.md global rule)
- Keep subject line under 72 characters
- Use imperative mood ("add" not "added" or "adds")
- Reference issue/card ID in body if relevant: `Implements kanban card #12`

## Branching Strategy

### Individual Feature Branches (Recommended)

Each agent works on an isolated feature branch for their component:

```
main (protected)
├── feature/skeleton          # Agent 1: ID 9 (foundational)
├── feature/config-loader     # Agent 1: ID 10
├── feature/state-queue       # Agent 2: ID 11, 14, 17
├── feature/plugin-protocol   # Agent 3: ID 12, 13
└── feature/dispatch-scheduler # Integration: ID 15, 16
```

**Branch naming:** `feature/<component-or-epic>`

**Workflow:**
1. Create branch from `main`: `git checkout -b feature/config-loader main`
2. Work on assigned tasks, commit frequently
3. Push to origin: `git push -u origin feature/config-loader`
4. When component is complete and tests pass, create PR to `main`
5. Merge via PR (fast-forward preferred, squash if commit history is messy)

**Why individual branches:**
- Components in `internal/` are loosely coupled
- Clear ownership and isolation
- Easy to review changes per component
- Can merge components independently as they complete

### Alternative: Stacked Branches (For Tightly Coupled Work)

Only use if components must be developed sequentially but need early visibility:

```
main
└── feature/foundation
    └── feature/scheduler (depends on foundation)
        └── feature/dispatch (depends on scheduler)
```

**Workflow:**
1. Base branch merges to main first
2. Rebase dependent branches onto main
3. More complex, use only if truly necessary

**Recommendation:** Stick with individual feature branches for Sprint 1.

## Parallel Work Assignment (3 Agents)

### Phase 0: Unblock Everything (Sequential, ~1-2 hours)

**Agent 1:**
- ✅ ID 24: Decide crash recovery policy (recommend Option B - SPEC semantics)
- ✅ ID 9: Go project skeleton (`go.mod`, directory structure, `main.go` stub)

**Why sequential:** Skeleton unblocks everything else. Decision affects implementation.

### Phase 1: Foundation Layer (Parallel, ~4-6 hours)

After skeleton is merged to `main`, all agents rebase and split work:

**Agent 1 - Configuration & Plugin System**
- Branch: `feature/config-plugin`
- ID 10: Config Loader + Env Interpolation
- ID 12: Plugin Discovery + Manifest Validation
- ID 13: Protocol v1 Codec (types in separate package)
- Deliverable: Can load config, discover plugins, encode/decode protocol v1 JSON

**Agent 2 - State & Queue**
- Branch: `feature/state-queue`
- ID 11: SQLite Schema Bootstrap
- ID 14: SQLite Work Queue (enqueue/dequeue/complete)
- ID 17: Plugin State Store + Shallow Merge
- ID 18: PID Lock (small, can bundle with state)
- Deliverable: Can persist jobs and plugin state, single-instance lock

**Agent 3 - Observability & Coordination**
- Branch: `feature/logging-scheduler`
- ID 19: Structured JSON Logging (logger utility)
- ID 15: Scheduler Tick Loop + Fuzzy Intervals
- ID 25: Crash Recovery Implementation (depends on decision from Phase 0)
- Deliverable: Scheduler can enqueue jobs on schedule, logging works, crash recovery handles orphans

**Integration Points:**
- Agent 1's protocol codec is used by Agent 3's scheduler (indirectly via queue)
- Agent 2's queue is used by Agent 3's scheduler
- All use Agent 3's logger

**Coordination:**
- Agents 1 & 2 can work completely independently
- Agent 3 needs queue interface from Agent 2 (can define interface early, implement against mock)

### Phase 2: Integration (Collaborative, ~3-4 hours)

**Agent 1 + Agent 2 - Dispatch Loop**
- Branch: `feature/dispatch` (new branch, both agents collaborate or hand off)
- ID 16: Dispatch Loop (spawns plugin, uses protocol codec from Agent 1, state from Agent 2)
- ID 20: Echo Plugin + E2E Runbook (validation)
- Deliverable: End-to-end loop works

**All Agents - Sprint Epic**
- ID 8: Sprint 1 MVP Core Loop (integration testing, final validation)

## Merge Order

Critical path for avoiding conflicts:

1. **Skeleton** (ID 9) - merge first, everyone rebases
2. **Foundation layer** - order doesn't matter, but suggested:
   - Logging (19) - others can use it immediately
   - Config (10) - needed by scheduler
   - State/Queue (11, 14, 17, 18) - needed by scheduler & dispatch
   - Plugin/Protocol (12, 13) - needed by dispatch
3. **Scheduler** (15) + Crash Recovery (25) - depends on queue
4. **Dispatch** (16) - depends on everything
5. **Echo Plugin** (20) - final validation

## Testing Strategy

Each agent is responsible for:
- Unit tests for their components (`_test.go` files)
- Table-driven tests (Go idiom)
- Tests must pass before PR: `go test ./...`

**Integration tests:** Deferred to Phase 2 (Echo Plugin validation)

## Common Development Commands

```bash
# Initial setup (Phase 0)
go mod init github.com/mattjoyce/senechal-gw
go mod tidy

# Build
go build -o senechal-gw ./cmd/senechal-gw

# Test all packages
go test ./...

# Test specific package
go test ./internal/config -v

# Test with coverage
go test -cover ./...

# Run linter (install first: brew install golangci-lint)
golangci-lint run

# Format code
go fmt ./...

# Check for common mistakes
go vet ./...
```

## Git Workflow Summary

```bash
# Agent starts work
git checkout main
git pull origin main
git checkout -b feature/my-component

# Regular commits
git add internal/config/
git commit -m "config: implement YAML parser"
git push -u origin feature/my-component

# Rebase before PR (if main has moved)
git fetch origin
git rebase origin/main
# Fix any conflicts
git push --force-with-lease origin feature/my-component

# Create PR via GitHub CLI
gh pr create --title "config: implement YAML parser with env interpolation" \
  --body "Implements kanban card #10..."

# After PR merged, clean up
git checkout main
git pull origin main
git branch -d feature/my-component
```

## Conflict Resolution

**If two agents touch the same file:**
- Components are designed to be isolated - this should be rare
- Use structured merge: keep both changes if they're in different functions
- If conflict is unavoidable, coordinate in PR review
- Last merger is responsible for resolving conflicts

**Common conflict zones:**
- `cmd/senechal-gw/main.go` - wire-up code (Phase 2 only, collaborate)
- `go.mod`/`go.sum` - run `go mod tidy` after merge
- Shared types/interfaces - define early, communicate changes

## Communication

**Before starting work:**
- Check kanban card is still `status: todo`
- Comment on card or create draft PR to claim work
- Update card `status: doing` when actively working

**When blocked:**
- If waiting on dependency, note it in card narrative
- Can continue with mock interfaces (define interface, implement later)

**When complete:**
- Update card `status: done`
- Ensure tests pass
- Create PR with clear description

## Decision Log

Track architectural decisions that affect multiple agents:

**ID 24 - Crash Recovery:** [Decision pending, to be made in Phase 0]
- Option A (MVP): Mark orphans as `dead`
- Option B (SPEC): Re-queue if under `max_attempts`
- **Chosen:** [TBD by Agent 1 in Phase 0]

Future decisions should be documented here before implementation.

## Go Project Standards

Per golang-pro skill and Go idioms:

- **Error handling:** Explicit returns, no `panic` in application code
- **Interfaces:** Small, focused (1-3 methods)
- **Packages:** Flat structure in `internal/`, avoid deep nesting
- **Naming:** Shorter is better in local scope, descriptive in package scope
- **Comments:** Package comment required, exported functions commented
- **Testing:** Table-driven tests, use `t.Helper()` for test utilities

## Tools to Install

```bash
# Linter (comprehensive, used by golangci-lint)
brew install golangci-lint

# GitHub CLI (for PR creation)
brew install gh
gh auth login

# Optional: Air for hot reload during development
go install github.com/air-verse/air@latest

# Optional: sqlc for type-safe SQL (if we want it for queue/state)
brew install sqlc
```

## Preflight Checklist

Before each agent starts:

- [ ] Go 1.21+ installed and in PATH (`go version`)
- [ ] Can build Go projects (`go build` works in some project)
- [ ] GitHub SSH key configured (`git push` works)
- [ ] Read SPEC.md sections relevant to your components
- [ ] Read CLAUDE.md for architecture overview
- [ ] Check dependency graph (this doc) for your assigned tasks
- [ ] Create feature branch from latest `main`
