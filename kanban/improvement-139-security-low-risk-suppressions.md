---
id: 139
status: done
priority: Low
blocked_by: []
tags: [improvement, security, cleanup]
---

# Security: Low-Risk Findings Cleanup / Suppressions

## Problem
Several gosec findings look low-risk or likely false positives (stderr output flagged as XSS, non-crypto RNG for jitter, int conversions on 64‑bit, unhandled encode/close errors). These create noise in reports unless we either harden or explicitly suppress with justification.

## Evidence (gosec.json excerpts)
```json
{
  "rule_id": "G404",
  "details": "Use of weak random number generator",
  "file": "internal/scheduler/scheduler.go",
  "line": "653"
}
{
  "rule_id": "G115",
  "details": "integer overflow conversion uintptr -> int",
  "file": "internal/lock/pidlock.go",
  "line": "32"
}
{
  "rule_id": "G705",
  "details": "XSS via taint analysis",
  "file": "cmd/ductile/main.go",
  "line": "109"
}
{
  "rule_id": "G104",
  "details": "Errors unhandled",
  "file": "internal/api/handlers.go",
  "line": "584"
}
```

## Proposed Direction
- Decide which findings are safe and annotate with `#nosec` and a justification.
- If any are easy to harden without behavior change (e.g., `crypto/rand` for jitter or explicit error handling), fix them instead of suppressing.

## Acceptance Criteria
- [x] Each low-risk finding is either fixed or suppressed with a documented justification.
- [x] gosec report no longer shows these items as “unknown risk.”

## Notes (assistant’s take)
I’d suppress G705 stderr “XSS” and G115 on `Flock` conversions (with a 64‑bit assumption), and only fix G404 if we want to be extra conservative. This is pure hygiene, not design work.

## Narrative
- 2026-02-28: Created to triage low-risk gosec findings; intentionally separated so higher-impact items can move first. (by @assistant)
- 2026-02-28: Added targeted #nosec annotations for jitter, stderr output, and Flock fd conversion; handled JSON encode error explicitly. (by @assistant)
