---
id: 127
status: todo
priority: High
blocked_by: []
assignee: ""
tags: [llm, cli, rfc-004, documentation, ux]
---

# #127: Externalize `skills` Manifest + AI-Native Content Model

When operating Ductile through an LLM, we need the `ductile skill` output to be easy to evolve without repetitive Go code edits, so we can keep the manifest highly structured, explicit, semantically anchored, and low-entropy.

## Goal

Make `ductile skill` content easy to tune by externalizing static copy into repository-managed template files and embedding them at build time, while enforcing a stronger AI-native structure for the rendered output.

## Proposed Approach

### 1. Externalize & Embed
- Move static manifest prose out of `cmd/ductile/main.go` into one or more Markdown templates in-repo.
- Use Go `//go:embed` to fold templates into the binary at build time.
- Keep runtime rendering deterministic (stable section order, stable sorting for commands/plugins/pipelines).

### 2. Structured Output Contract
- Define an explicit output schema/contract for `skill` sections (core utilities, atomic plugin skills, pipeline skills).
- Include consistent per-skill descriptors where possible:
  - intent
  - invocation surface (CLI/API)
  - required scopes/tier
  - expected input shape
  - expected output shape
  - side effects / risk
  - failure modes

### 3. AI-Quality Hardening
- Make content semantically anchored and hierarchical:
  - canonical section IDs/titles
  - concise, non-overlapping language
  - predictable command naming and endpoint forms
- Add tests that catch regressions in structure and required fields.
- Add golden/snapshot test(s) to keep output shape stable and reviewable.

## Acceptance Criteria
- [ ] `ductile skill` static copy lives in embedded template file(s), not hardcoded print chains.
- [ ] Build still produces a self-contained binary (no runtime file dependency for templates).
- [ ] Output remains deterministic across runs for identical config.
- [ ] A documented structure exists for rendered skill entries (fields/sections expected by AI operators).
- [ ] Tests validate structural invariants and snapshot output for at least one fixture config.
- [ ] `RFC-004` alignment is explicit in either code comments or docs (operator utilities + skill affordances + dry-run guidance language).

## Narrative
- 2026-02-25: Card created to externalize `skills` output and raise content quality toward an AI-native, low-entropy operator manifest. (by @assistant)
