---
id: 94
status: done
priority: High
blocked_by: []
assignee: "@gemini"
due_date: 2026-02-14
tags: [refactor, branding]
---

# Rename Project: Senechal-gw -> Ductile

Global rename of the project from "Senechal-gw" to "Ductile".

## Job Story
When the project identity shifts, I want to rename all references to "Senechal-gw" to "Ductile", so the codebase and documentation reflect the new brand consistently.

## Acceptance Criteria
- [x] Documentation updated (README.md, SPEC.md, RFCs, TEAM.md, etc.)
- [x] Go module path updated in `go.mod`
- [x] Internal import paths updated in all `.go` files
- [x] Binary entrypoint moved from `cmd/senechal-gw` to `cmd/ductile`
- [x] Docker configurations updated (`Dockerfile`, `docker-compose.yml`)
- [x] CLI help text and log messages updated
- [x] Existing Kanban cards updated (where relevant for clarity)

## Order of Operations
1. **Phase 1: Docs & Metadata** (README, SPEC, RFCs, Kanban) - COMPLETED
2. **Phase 2: Go Module & Imports** (`go.mod`, all `.go` files) - COMPLETED
3. **Phase 3: Filesystem & Build** (`cmd/` rename, Docker, Makefile/CLAUDE.md) - COMPLETED
4. **Phase 4: Runtime Strings** (Logs, CLI help, default paths) - COMPLETED

## Narrative
- 2026-02-14: Initiated renaming task per user request. Defined 4-phase plan. (by @gemini)
- 2026-02-14: Completed global rename. All documentation, Go imports, filesystem paths, and Docker configs updated to "Ductile". Verified with `go build` and `go test ./...`. All tests passed. (by @gemini)
