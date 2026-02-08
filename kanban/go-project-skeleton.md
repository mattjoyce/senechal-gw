---
id: 9
status: done
priority: High
blocked_by: []
tags: [sprint-1, mvp, go]
---

# Go Project Skeleton

When starting a new integration gateway project, I want a minimal Go scaffold that matches the SPEC layout, so I can iterate on core components without rework.

## Acceptance Criteria
- Project layout matches SPEC section "Project Layout" (`cmd/`, `internal/`, `plugins/`, `config.yaml` sample or fixture).
- `go.mod` present and `go test ./...` runs (even if tests are minimal at first).
- `senechal-gw` builds and `senechal-gw start --help` works.

## Narrative
- **2026-02-08: Skeleton complete.** Created go.mod, directory structure (cmd/, internal/*/doc.go, plugins/echo/), minimal main.go with CLI stub (start/version/help commands), sample config.yaml, and echo test plugin. All acceptance criteria met: builds successfully, `go test ./...` runs, CLI help works.

