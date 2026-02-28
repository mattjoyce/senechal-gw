---
id: 137
status: done
priority: High
blocked_by: []
tags: [improvement, security, sqlite, queue]
---

# Security: SQL Where-Clause Hardening

## Problem
`internal/queue/queue.go` builds SQL queries with `fmt.Sprintf` and interpolates `whereClause`. If `whereClause` is built from untrusted input, this becomes a SQL injection risk. Even if it’s currently built from a controlled builder, the code shape looks risky and will keep tripping scanners.

## Evidence (gosec.json excerpts)
```json
{
  "rule_id": "G201",
  "details": "SQL string formatting",
  "file": "internal/queue/queue.go",
  "line": "422"
}
{
  "rule_id": "G201",
  "details": "SQL string formatting",
  "file": "internal/queue/queue.go",
  "line": "428-435"
}
```

## Proposed Direction
- Confirm how `whereClause` is built (inputs, whitelist, allowed fields/operators).
- If the clause is fully whitelisted, document it and add a small guardrail (e.g., a builder with allowed columns + operators).
- If any user input flows into `whereClause`, refactor to parameterized queries.

## Acceptance Criteria
- [x] Document the `whereClause` construction path and trust model.
- [x] Ensure only whitelisted columns/operators can appear in the where clause.
- [x] Either: fully parameterize the query OR add a `// #nosec` with a clear justification.

## Notes (assistant’s take)
This *could* be design-breaking if filters are user-controlled. If they are config-controlled only, it’s still worth formalizing a builder so the trust model is explicit and gosec stops screaming.

## Narrative
- 2026-02-28: Created after gosec flagged SQL string formatting; needs explicit query builder/whitelist decision. (by @assistant)
- 2026-02-28: Replaced fmt.Sprintf assembly with fixed-fragment builder and parameterized args; documented trust model in code. (by @assistant)
