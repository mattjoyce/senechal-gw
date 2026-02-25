---
id: 127
status: todo
priority: High
blocked_by: []
assignee: ""
tags: [llm, cli, rfc-004, documentation, ux, lx]
---

# #127: Externalize `skills` Manifest + Strong Semantic Anchoring (AI-Native)

When operating Ductile through an LLM, the `ductile skills` surface must behave as a stable, low-entropy capability registry — not just formatted help text. This card formalizes the move toward a strongly semantically anchored, explicit, AI-native manifest model.

---

## Goal

Transform `ductile skills` into a deterministic, schema-aligned capability manifest that:

- Is easy to evolve without repetitive Go edits
- Is embedded at build time (no runtime template dependency)
- Is structurally stable across versions
- Minimizes ambiguity and inference for LLM operators
- Provides explicit semantic anchors for safe autonomous reasoning

---

## Design Principles (LX-Oriented)

### 1. Strong Semantic Anchoring

All rendered entities must be explicitly typed and structured.

Avoid implicit meaning based on formatting or position.

Examples of required anchors:

- `entity: plugin | command | pipeline | utility`
- `mutates_state: true|false`
- `idempotent: true|false`
- `retry_safe: true|false`
- `invocation` (transport + method + path template)
- `input_schema`
- `output_schema`

The LLM should never need to infer safety or invocation shape from prose.

---

### 2. Low Entropy, Non-Overlapping Language

Content must be:

- Concise
- Non-redundant
- Non-poetic
- Deterministic in ordering

No repeated explanations across sections.
No descriptive drift between plugins.
No variable phrasing for identical semantics.

---

### 3. Deterministic Structure

- Stable section ordering
- Stable sorting of plugins/commands
- Canonical section IDs/titles
- Snapshot-testable output

For identical config → identical output bytes.

---

## Proposed Implementation

### 1. Externalize & Embed

- Move static `skills` template content out of `cmd/ductile/main.go`.
- Store in versioned template file(s) (e.g. `templates/skills.v1.md`).
- Use Go `//go:embed` to produce a self-contained binary.
- No runtime template file dependency.

---

### 2. Versioned Skills Schema

Define and document:

`ductile.skills.schema.v1`

This schema must describe:

- Allowed entity types
- Required semantic fields per entity
- Structural invariants
- Invocation contract

The Markdown output becomes a structured rendering of this schema.

---

### 3. Structured Output Contract

Each skill entry should expose, where applicable:

- intent
- entity type
- invocation surface (CLI/API)
- required scopes or tier
- mutability semantics
- idempotency + retry safety
- expected input shape
- expected output shape
- side effects
- emitted events
- failure modes

Fields should be explicit rather than implied by prose.

---

### 4. AI-Quality Hardening

- Enforce required semantic fields via tests
- Add golden/snapshot tests for representative config
- Fail tests if structural anchors are missing
- Ensure stable rendering order

---

## Acceptance Criteria

- [ ] `ductile skills` static copy lives in embedded template file(s)
- [ ] Output aligns with a documented `ductile.skills.schema.v1`
- [ ] All plugin commands include explicit mutability + idempotency semantics
- [ ] Invocation contract is rendered explicitly (no implicit path guessing)
- [ ] Output is byte-deterministic for identical config
- [ ] Snapshot test covers at least one multi-plugin fixture
- [ ] Structural tests validate required semantic anchors
- [ ] RFC-004 alignment is explicitly referenced

---

## Strategic Framing

`skills` is not help text.

It is the machine-readable capability surface of the gateway.

This card elevates it from formatted CLI documentation to:

> A stable, semantically anchored capability registry optimized for autonomous operators.

---

## Narrative

- 2026-02-25: Card expanded to formalize strong semantic anchoring and low-entropy AI-native manifest direction. (by @assistant)
