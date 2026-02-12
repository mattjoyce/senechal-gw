---
id: 86
status: todo
priority: High
blocked_by: []
tags: [improvement, cli, release, versioning]
---

# IMPROVEMENT: Add `--version` and define versioning strategy

## Description

The CLI has a `version` command, but we need a clear operator experience for `--version` and a documented strategy for how versions are generated and managed across builds/releases.

## Job Story

When I run `senechal-gw --version`, I want reliable version/build metadata and a defined versioning policy, so I can identify exactly what binary is running and how it maps to source and releases.

## Acceptance Criteria

- CLI supports both `senechal-gw version` and `senechal-gw --version` (and `-v` only if it does not conflict with existing semantics).
- Version output includes:
  - semantic version
  - git commit (short SHA) when available
  - build date/time (UTC) when available
- `--json` output option exists for machine-readable version metadata.
- A documented versioning strategy is added (e.g., SemVer policy, prerelease tags, release tagging process).
- Build/release process documents how version/build metadata is injected (default local/dev behavior included).
- Unit tests cover CLI version flag behavior and output shape.

## Notes

- Avoid ambiguity with `-v` because it already means `--verbose` for subcommands.
- Keep local developer builds usable even when git metadata is unavailable.

## Narrative
- 2026-02-12: Card created to formalize both UX (`--version`) and release/process versioning strategy before broader CLI/operator automation. (by @assistant)
