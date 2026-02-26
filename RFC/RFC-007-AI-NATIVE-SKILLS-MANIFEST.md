# RFC-130: AI-Native Skills Manifest

Status: Draft  
Date: 2026-02-25

---

## 1. Summary

This RFC formalizes "ductile skills" as the authoritative machine-readable
capability registry of the Ductile gateway.

It establishes strong semantic anchoring, deterministic rendering, and
low-entropy structural guarantees to support autonomous LLM operators.

---

## 2. Motivation

Traditional CLI help text optimizes for human readability.

Ductile operates in AI-mediated environments where agents:

- Enumerate capabilities
- Reason about safety
- Construct invocation requests
- Plan multi-step orchestration

Implicit semantics increase entropy and planning variance.

---

## 3. Principles

### 3.1 Strong Semantic Anchoring

All entities must be explicitly typed.
All safety properties must be explicit.
No reliance on prose for operational guarantees.

### 3.2 Low Entropy Output

- Stable section ordering
- Non-overlapping language
- Deterministic rendering
- No redundant explanation

### 3.3 Versioned Contract

The output aligns with ductile.skills.schema.v1.
Breaking changes require a schema bump.

---

## 4. Non-Goals

- Not a marketing surface
- Not free-form documentation
- Not a replacement for OpenAPI

---

## 5. Architectural Position

"ductile skills" becomes the root capability graph for:

- Plugin discovery
- Pipeline discovery
- Safety reasoning
- Automated SDK generation
- Agent planning

---

## 6. Consequences

### Positive

- Deterministic AI behavior
- Reduced orchestration variance
- Easier snapshot testing
- Clear evolution model

### Tradeoffs

- Increased verbosity
- Stricter schema discipline
- Reduced prose flexibility

---

## 7. Adoption Plan

1. Implement schema v1 in documentation
2. Align ductile skills --json output
3. Add snapshot and structural tests
4. Migrate templates to embedded versioned files

---

## 8. Future Work

- Schema v2: richer execution semantics
- Machine-readable validation tooling
- Capability diff tooling between gateway instances

