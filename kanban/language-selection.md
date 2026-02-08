---
id: 5
status: done
priority: Normal
blocked_by: []
tags: [tooling, language]
---

# Language Selection

Evaluate Go, Rust, Python, TypeScript for the implementation.

## Acceptance Criteria
- Candidates evaluated against project requirements (lightweight, modular, service-oriented)
- Concurrency model suitability assessed
- Ecosystem/library support for connectors evaluated
- Deployment simplicity considered (single binary vs runtime)
- Decision documented with rationale

## Narrative
- 2026-02-08: Created during initial project discussion. User wants something light — existing heavy integration servers are a pain point. Language choice will heavily influence the "feel" of the system. (by @assistant)
- 2026-02-08: Decision: **Go** for the core. Rationale: single compiled binary for easy deployment, good concurrency primitives for the dispatch loop, subprocess spawning is natural. Plugins are polyglot (Python, shell, anything) — so language ecosystem for connectors is not constrained by core choice. (by @assistant)
