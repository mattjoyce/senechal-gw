# Agent Instructions

## Issue Tracking

Use **bd (beads)** for all issue tracking. Do not use markdown TODOs or other trackers.
Run `bd onboard` for workflow context.

Quick reference:

```bash
bd ready --json
bd show <id> --json
bd update <id> --status in_progress --json
bd close <id> --reason "Done" --json
```

> This file governs all agent behaviour in this codebase. Read it fully before writing any code.

---

## Project Identity

**Ductile** is a Go-based integration platform. It is research-grade but production-disciplined. Code must be readable, composable, and honest about what it does.

---

## Non-Negotiable Rules

These are hard stops. If you cannot satisfy them, stop and report back rather than work around them.

1. **Never mock what you can use for real.** If a real implementation can be used in a test, use it. Mocks are permitted only when the dependency is genuinely external and uncontrollable (e.g. a third-party API with no local equivalent). Document why a mock was necessary with a `// MOCK: reason` comment.

2. **Never fix a bad abstraction — report it.** If the existing structure makes the task difficult or wrong, stop and describe the problem. Do not paper over it.

3. **Every test must test observable behaviour, not implementation.** Test what the function does, not how it does it. If the test would still pass after deleting the implementation and replacing it with a stub, it is a bad test.

4. **No `git push` without explicit human permission.** Commits are fine. Pushing requires a direct human instruction in the current session.

5. **No new files without a clear home.** Follow the existing package structure. If a new package is warranted, propose it — don't create it unilaterally.

---

## Go Standards

### Types and Interfaces

- Define interfaces at the point of use, not the point of implementation
- Keep interfaces small — prefer single-method interfaces
- Return concrete types from constructors; accept interfaces as parameters
- Use `errors.Is` / `errors.As` for error inspection, never string matching

### Error Handling

- Every error must be handled or explicitly ignored with a comment explaining why
- Wrap errors with context: `fmt.Errorf("ductile/pkg: doing X: %w", err)`
- No `panic` except in `main` or `init` for truly unrecoverable setup failures
- No `log.Fatal` outside of `main`

### Style

- `gofmt` and `goimports` are mandatory — code must be formatted before committing
- `golangci-lint` must pass with the project config (see `.golangci.yml`)
- Max function length: 60 lines. If longer, decompose.
- Max file length: 400 lines. If longer, split by responsibility.
- Exported symbols must have doc comments

### Concurrency

- Never share memory without synchronisation — use channels or mutexes explicitly
- Document goroutine ownership: who starts it, who stops it, what it owns
- Always handle context cancellation in long-running operations

---

## Testing Philosophy

Ductile uses a **staged testing strategy**:

1. **Fast tests for branch development**
   - default inner loop during implementation
   - optimized for quick feedback
2. **Docker-backed tests for system confidence**
   - used selectively during development when runtime realism matters
   - required before merge
3. **Full validation on `main` after merge**
   - protects trunk health on the merged state

Testing orchestration belongs in repository tooling under `scripts/`, not in the `ductile` CLI.

### The Anti-Mocking Rule in Detail

Go makes it easy to write interface mocks. Resist this. The pattern to follow:

```
// GOOD — uses a real implementation
func TestRouterDispatch(t *testing.T) {
    r := NewRouter()
    r.Register("ping", PingHandler{})
    
    resp, err := r.Dispatch(context.Background(), "ping", nil)
    require.NoError(t, err)
    assert.Equal(t, "pong", resp)
}

// BAD — mocks the handler, tests nothing real
func TestRouterDispatch(t *testing.T) {
    mockHandler := &MockHandler{}
    mockHandler.On("Handle", ...).Return("pong", nil)
    ...
}
```

### Canonical Test Commands

Use the repository test scripts as the source of truth:

- `scripts/test-fast` — fast branch-development validation
- `scripts/test-docker` — Docker-backed fixture validation
- `scripts/test-premerge` — merge-grade validation
- `scripts/test-main` — post-merge trunk validation

Current expected meanings:

- `scripts/test-fast` should run the standard fast suite, initially `go test ./...`
- `scripts/test-docker` should run the fixture-driven Docker harness
- `scripts/test-premerge` should compose `scripts/test-fast`, `golangci-lint run ./...`, and `scripts/test-docker`
- `scripts/test-main` should initially be at least as strong as `scripts/test-premerge`

If a `Makefile` wrapper exists, it should delegate to these scripts rather than duplicate logic.

### Fast vs Docker Test Boundaries

Fast tests should own:

- pure logic
- deterministic unit behaviour
- parser and validator correctness
- router, state, and queue integration using real SQLite where helpful
- day-to-day confidence during branch work

Docker-backed tests should own:

- runtime system behaviour
- service boot with real config
- restart and recovery flows
- network ingress behaviour
- realistic end-to-end operator-facing scenarios

Do not duplicate the entire Go test suite inside Docker.

### Docker Harness Directives

The Docker-backed harness must be:

- fixture-driven
- Docker Compose based
- explicit about readiness checks rather than relying on sleeps
- focused on black-box, high-value scenarios
- capable of automatic artifact capture on failure

The current first-wave fixture set is:

- `webhook-ingress`
- `scheduler-recovery`
- `api-e2e`

These scenarios come from the recent test harness work and should remain the baseline for Docker-backed coverage.

When extending the harness:

- prefer focused fixtures under `test/fixtures/docker/`
- keep local and CI entry points aligned through the same scripts
- support running all fixtures or a named fixture
- keep wave-1 fixtures small, stable, and diagnostically useful

On Docker-backed failures, retain predictable artifacts under a path such as `test-artifacts/docker/<timestamp>/<fixture>/` including logs, scenario output, relevant config, and responses/DB snapshots where applicable.

### Test Structure

- Use table-driven tests for any function with more than two interesting cases
- Use `t.Helper()` in assertion helpers
- Name subtests descriptively: `t.Run("returns error when config is missing", ...)`
- Test files live beside the code they test (`foo_test.go` next to `foo.go`)
- Integration tests go in `_test` packages with a build tag: `//go:build integration`

### Branch, Pre-Merge, and Main Expectations

- The default branch-development loop should stay fast and should not require Docker
- Lint is not required in the default fast inner loop
- Before merge, the branch must pass fast tests, lint/static checks, and Docker-backed validation
- `main` must receive a full validation pass after merge
- Failures on `main` are trunk-health issues and should be triaged promptly

### Coverage

- Aim for >80% coverage on business logic packages
- 100% pass rate required before any work is considered complete
- If a test is flaky, fix or delete it — do not skip it

### Test Data and Fixtures

- Use real test fixtures in `testdata/` directories
- For database tests, use an in-memory SQLite or a real test DB, not a mock repository
- For HTTP tests, use `httptest.NewServer` — not a mocked HTTP client
- For Docker/system tests, use real fixture directories under `test/fixtures/docker/` rather than ad hoc local setup

---

## Spec Discipline

Before writing code for any non-trivial task, produce a brief spec in your working notes covering:

- What files will be touched
- What the expected input/output behaviour is
- What edge cases exist
- What tests will verify correctness

Do not begin implementation until the spec is clear. If the spec cannot be written clearly, the task is not well-defined — report back.

---

## Scope Discipline

Each agent session has a defined scope. If you encounter something broken or missing outside that scope:

- Note it in a `// TODO(out-of-scope):` comment
- Do not fix it in this session
- Report it at the end of the session

Do not expand scope without explicit instruction.

---

## Security (gosec)

All code must pass `gosec` review before a session is considered complete. This is not optional.

### Running gosec

```bash
gosec ./...
```

For a detailed report with rule explanations:

```bash
gosec -fmt text -severity medium ./...
```

### Remediation Rules

**Fix, don't suppress.** If gosec flags an issue, fix the underlying problem. Do not add `#nosec` annotations as a first response.

`#nosec` requires **explicit human authorisation before use**. Do not add it unilaterally.

If you believe a `#nosec` is warranted:
1. Stop. Do not add the annotation.
2. Report the finding, the rule ID, and your reasoning for why it is a false positive.
3. Wait for explicit written approval before proceeding.

Once authorised, the annotation must include the rule ID and the approved justification:
`#nosec G304 -- AUTHORISED: path is constructed from validated config, not user input`

### Common Issues and Correct Fixes

**G104 — Errors unhandled**
```go
// BAD
f.Close()

// GOOD
if err := f.Close(); err != nil {
    return fmt.Errorf("closing file: %w", err)
}
```

**G304 — File path from variable**
```go
// BAD
f, err := os.Open(userInput)

// GOOD — validate and clean the path first
clean := filepath.Clean(userInput)
if !strings.HasPrefix(clean, allowedRoot) {
    return errors.New("path outside allowed root")
}
f, err := os.Open(clean)
```

**G401/G501 — Weak crypto (MD5, SHA1)**
```go
// BAD
h := md5.New()

// GOOD
h := sha256.New()
```

**G501 — Insecure random**
```go
// BAD
n := rand.Intn(100)

// GOOD (for security-sensitive use)
b := make([]byte, 8)
_, err := crypto_rand.Read(b)
```

**G501 — TLS minimum version**
```go
// BAD
tls.Config{}

// GOOD
tls.Config{MinVersion: tls.VersionTLS12}
```

### Severity Triage

| Severity | Action |
|----------|--------|
| High | Must fix before session end, no exceptions |
| Medium | Must fix or provide written justification |
| Low | Fix if straightforward; document if deferred |

### Report at Session End

Include in your session summary:
- gosec finding count before and after
- Any `#nosec` annotations added and their justification
- Any medium/low findings deliberately deferred and why

---

## Quality Gates (must pass before session is complete)

```bash
gofmt -l .          # must produce no output
goimports -l .      # must produce no output  
golangci-lint run   # must pass
gosec ./...         # must pass or all findings documented
go test ./...       # must pass, 0 failures
go vet ./...        # must pass
```

Run these in order. If any fail, fix before considering the task done.

---

## What to Report at Session End

Summarise:

1. What was done
2. What files were changed and why
3. Any out-of-scope issues noted
4. Any mocks used and why they were necessary
5. Test coverage delta (before/after if measurable)
6. Anything uncertain or that warrants human review

<!-- BEGIN BEADS INTEGRATION -->
## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL issue tracking. Do NOT use markdown TODOs, task lists, or other tracking methods.

### Why bd?

- Dependency-aware: Track blockers and relationships between issues
- Git-friendly: Dolt-powered version control with native sync
- Agent-optimized: JSON output, ready work detection, discovered-from links
- Prevents duplicate tracking systems and confusion

### Quick Start

**Check for ready work:**

```bash
bd ready --json
```

**Create new issues:**

```bash
bd create "Issue title" --description="Detailed context" -t bug|feature|task -p 0-4 --json
bd create "Issue title" --description="What this issue is about" -p 1 --deps discovered-from:bd-123 --json
```

**Claim and update:**

```bash
bd update <id> --claim --json
bd update bd-42 --priority 1 --json
```

**Complete work:**

```bash
bd close bd-42 --reason "Completed" --json
```

### Issue Types

- `bug` - Something broken
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature with subtasks
- `chore` - Maintenance (dependencies, tooling)

### Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (default, nice-to-have)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

### Workflow for AI Agents

1. **Check ready work**: `bd ready` shows unblocked issues
2. **Claim your task atomically**: `bd update <id> --claim`
3. **Work on it**: Implement, test, document
4. **Discover new work?** Create linked issue:
   - `bd create "Found bug" --description="Details about what was found" -p 1 --deps discovered-from:<parent-id>`
5. **Complete**: `bd close <id> --reason "Done"`

### Auto-Sync

bd automatically syncs via Dolt:

- Each write auto-commits to Dolt history
- Use `bd dolt push`/`bd dolt pull` for remote sync
- No manual export/import needed!

### Important Rules

- ✅ Use bd for ALL task tracking
- ✅ Always use `--json` flag for programmatic use
- ✅ Link discovered work with `discovered-from` dependencies
- ✅ Check `bd ready` before asking "what should I work on?"
- ❌ Do NOT create markdown TODO lists
- ❌ Do NOT use external issue trackers
- ❌ Do NOT duplicate tracking systems

For more details, see README.md and docs/QUICKSTART.md.

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd dolt push
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds

<!-- END BEADS INTEGRATION -->
