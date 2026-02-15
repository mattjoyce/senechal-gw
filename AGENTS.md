# Repository Guidelines

Ductile (`ductile`) is currently a docs-first repository with a Go-based implementation planned/underway. Use the spec as the source of truth and keep changes small, reviewable, and tied to an MVP milestone.

## Project Structure & Module Organization

- `docs/ARCHITECTURE.md`: Unified, buildable specification (architecture, protocols, queue/state semantics).
- `MVP.md`: Minimal end-to-end slice to prove the core loop (config -> queue -> plugin -> SQLite state).
- `RFC/`: Design proposals and critiques; use for decisions that materially change `SPEC.md`.
- `kanban/`: Work items/runbooks (cards include frontmatter like `id:` and `status:`).
- Planned implementation layout (per spec/kanban): `cmd/`, `internal/`, `plugins/`, `config.yaml`, `data/` (SQLite state; ignored by `.gitignore`).

## Build, Test, and Development Commands

After the Go skeleton exists (`go.mod`, `cmd/`, etc.), prefer these commands:

```bash
go test ./...           # run unit tests
go build ./cmd/ductile
./ductile start --config ./config.yaml
./ductile system monitor # launch real-time TUI dashboard
```

For fast repo-wide search (docs + code):

```bash
rg "dispatch loop|protocol|dedupe_key"
```

## Coding Style & Naming Conventions

- Go: run `gofmt` on changed files; keep packages small and cohesive under `internal/`.
- Commands: the primary binary should live at `cmd/ductile/`.
- Plugins: `plugins/<name>/manifest.yaml` plus an executable entrypoint (e.g. `run.sh`, `run.py`).
- Config: use `config.yaml` and `${ENV_VAR}` interpolation; do not hardcode secrets in YAML.

## Testing Guidelines

- Go unit tests live next to code as `*_test.go` (table-driven tests preferred).
- For end-to-end validation, follow/extend `kanban/echo-plugin-e2e-runbook.md`.
- Avoid committing local state: SQLite files under `data/` are ignored.

## Commit & Pull Request Guidelines

- Current git history uses short, descriptive subjects with optional detail after a colon (e.g. `Initial commit: ...`). Keep the first line under ~72 chars.
- PRs should include: what/why, the impacted spec sections (`docs/ARCHITECTURE.md` headings), and the linked work item (`kanban/*.md` `id:` if applicable).
- If you change protocol/config/SQLite behavior, update `docs/ARCHITECTURE.md` (and `MVP.md` if it affects the MVP) and include a minimal verification checklist (commands + expected output/logs).

## Security & Configuration Tips

- Never commit secrets (`.env`, `*.secret` are ignored); prefer environment variables and local `.env.local`.
- Treat plugin subprocesses as untrusted: validate inputs/outputs and ensure timeouts match `MVP.md`.

