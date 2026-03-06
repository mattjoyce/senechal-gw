# Agent Instructions

This project uses **bd** (beads) for issue tracking. Run `bd onboard` to get started.

## Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --status in_progress  # Claim work
bd close <id>         # Complete work
bd sync               # Sync with git
```

<!-- BEGIN BEADS INTEGRATION -->
## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL issue tracking. Do NOT use markdown TODOs, task lists, or other tracking methods.

### Why bd?

- Dependency-aware: Track blockers and relationships between issues
- Git-friendly: Auto-syncs to JSONL for version control
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
bd update bd-42 --status in_progress --json
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
2. **Claim your task**: `bd update <id> --status in_progress`
3. **Work on it**: Implement, test, document
4. **Discover new work?** Create linked issue:
   - `bd create "Found bug" --description="Details about what was found" -p 1 --deps discovered-from:<parent-id>`
5. **Complete**: `bd close <id> --reason "Done"`

### Auto-Sync

bd automatically syncs with git:

- Exports to `.beads/issues.jsonl` after changes (5s debounce)
- Imports from JSONL when newer (e.g., after `git pull`)
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

<!-- END BEADS INTEGRATION -->

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

### Test Structure

- Use table-driven tests for any function with more than two interesting cases
- Use `t.Helper()` in assertion helpers
- Name subtests descriptively: `t.Run("returns error when config is missing", ...)`
- Test files live beside the code they test (`foo_test.go` next to `foo.go`)
- Integration tests go in `_test` packages with a build tag: `//go:build integration`

### Coverage

- Aim for >80% coverage on business logic packages
- 100% pass rate required before any work is considered complete
- If a test is flaky, fix or delete it — do not skip it

### Test Data and Fixtures

- Use real test fixtures in `testdata/` directories
- For database tests, use an in-memory SQLite or a real test DB, not a mock repository
- For HTTP tests, use `httptest.NewServer` — not a mocked HTTP client

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
