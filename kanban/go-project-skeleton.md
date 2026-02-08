---
id: 9
status: todo
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

