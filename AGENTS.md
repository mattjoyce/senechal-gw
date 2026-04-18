# Agent Instructions

This file is intentionally short. Keep detailed workflows in focused docs such as `docs/TESTING.md`, not here.

## Project Identity

Ductile is a Go-based integration platform. It is research-grade but production-disciplined. Code should be readable, composable, and explicit about what it does.

## Issue Tracking

Use `bd` for issue tracking. Do not use markdown TODO lists or other parallel trackers. Run `bd onboard` when workflow context is needed.

## Core Rules

- Do not mock what can be exercised with a real local implementation.
- Test observable behavior, not implementation details.
- Handle every error or explicitly document why it is safe to ignore.
- Respect context cancellation in long-running operations.
- Do not push without explicit human permission in the current session.
- Do not expand scope silently; report unrelated problems instead of fixing them opportunistically.
- If an abstraction is making the work difficult or misleading, report that rather than papering over it.

## Go Defaults

- Use established standard-library tools first: `testing`, `net/http`, `httptest`, `errors`, `fmt.Errorf(... %w ...)`, `database/sql`, and `go mod`.
- Prefer project-appropriate common libraries when needed: `cobra` or `flag` for CLI, `viper` plus YAML or `envconfig` for config, `log/slog` for logging, `huh` for prompts, and `bubbletea` for full TUI flows.
- Put binaries in `cmd/<name>/main.go`; keep `main` thin and delegate to `Run(ctx) error`.
- Group packages by concern rather than MVC layers, and put non-public code under `internal/`.
- Define interfaces where they are used, keep them small, accept interfaces, and return concrete types from constructors.

## Go Error And Concurrency Rules

- Wrap propagated errors with useful context.
- Use `errors.Is` and `errors.As`; do not inspect errors by string matching.
- Do not `panic` except for unrecoverable startup failures.
- Do not use `log.Fatal` outside `main`.
- Do not log and return the same error.
- Put `context.Context` first for blocking or cancellable work.
- Use context cancellation for shutdown.
- Use either mutexes or channels for a datum, not both.
- Use `defer mu.Unlock()` immediately after `mu.Lock()`.

## Go Style

- `gofmt` and `goimports` are mandatory.
- `golangci-lint` must pass for merge-grade work.
- Refactor functions longer than roughly 60 lines and files longer than roughly 400 lines.
- Exported names need doc comments.
- Avoid package stutter, vague names, and clever abstractions that hide behavior.

## Go Testing

- Prefer real components over mocks; mock only truly uncontrollable external systems.
- Use real SQLite or a real test database where practical.
- Use `httptest.NewServer` for HTTP behavior.
- Use table-driven tests when there are multiple meaningful cases.
- Keep tests beside the code they exercise.
- Race test failures and flaky tests block merge readiness.

## Go Security

- `gosec` must pass for merge-grade work.
- `#nosec` requires explicit human approval, the rule ID, and a concrete justification.
- Never log secrets.
- Validate external input.
- Use `html/template` for HTML output.
- Use `//go:embed` only for small, static, versioned assets; prefer `embed.FS` for file sets.

## Testing

Ductile uses staged validation:

- Fast branch work: `scripts/test-fast` / `go test ./...`
- Docker-backed system confidence: `scripts/test-docker`
- Merge readiness: `scripts/test-premerge`
- Main branch validation: `scripts/test-main`

Testing orchestration belongs in repository scripts, not in the `ductile` CLI. Docker-backed tests should be fixture-driven, readiness-driven, black-box, and should capture useful failure artifacts.

## Quality

Before considering code work complete, run the relevant quality gates for the scope changed. For Go changes, the expected gates are formatting, lint/static checks, security review, tests, and vet. If a gate cannot be run or fails for an unrelated reason, report that clearly.

Use `gosec` findings as real findings. Fix them where possible. Do not add `#nosec` without explicit human authorization, and include the rule ID and justification if authorization is given.

For merge-grade Go work, the expected checks are:

```bash
gofmt -l .
goimports -l .
go vet ./...
golangci-lint run ./...
gosec -severity medium ./...
go test -race -count=1 -shuffle=on -vet=all ./...
```

## Session Summary

At handoff, summarize what changed, which files changed and why, what was verified, any mocks used, any out-of-scope issues found, and anything that needs human review.
