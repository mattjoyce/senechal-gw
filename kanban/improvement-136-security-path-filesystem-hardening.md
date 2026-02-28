---
id: 136
status: todo
priority: High
blocked_by: []
tags: [improvement, security, filesystem, config]
---

# Security: Path Handling + Archive Restore Hardening

## Problem
Gosec flags multiple path traversal/file inclusion issues across config loading, backup/restore, and workspace operations. These findings are only *real vulnerabilities* if any of these paths can be influenced by untrusted input (env/CLI/config). We need to either harden paths or explicitly document and enforce a trusted-input model.

## Evidence (gosec.json excerpts)
```json
{
  "rule_id": "G703",
  "details": "Path traversal via taint analysis",
  "file": "internal/config/loader.go",
  "line": "120"
}
{
  "rule_id": "G304",
  "details": "Potential file inclusion via variable",
  "file": "internal/config/loader.go",
  "line": "280"
}
{
  "rule_id": "G122",
  "details": "Filesystem operation in filepath.Walk/WalkDir callback uses race-prone path",
  "file": "internal/workspace/fs_manager.go",
  "line": "210"
}
{
  "rule_id": "G110",
  "details": "Potential DoS vulnerability via decompression bomb",
  "file": "cmd/ductile/config_manage.go",
  "line": "2285"
}
```

## Proposed Direction
Choose one of the two stances below and implement consistently:

1) **Trusted input model** (lowest effort):
   - Document that config/paths/archives are operator-controlled only.
   - Add explicit validation and comments to codify this assumption.
   - Add `// #nosec` annotations with justifications for the specific findings.

2) **Hardening model** (safer by default):
   - Scope file access under a fixed root (Go `os.Root` where available) or equivalent path sanitization.
   - Validate resolved paths are within the expected config directory.
   - Add size limits and file count limits when restoring archives (tar extraction guardrails).

## Acceptance Criteria
- [ ] Decide trusted-input vs hardening model for config + archive paths.
- [ ] For chosen model, update code and/or docs accordingly.
- [ ] No unresolved G703/G304/G122/G110 findings without a documented justification.

## Notes (assistant’s take)
I don’t think this is design-breaking, but it *is* policy-setting. If we ever allow remote config changes or user-provided archive uploads, the trusted-input stance becomes unsafe. If you want Ductile to be “operator-only,” document it loudly and suppress the findings. If you want it to be robust by default, we should treat this as a hardening milestone.

## Narrative
- 2026-02-28: Created after gosec scan flagged path traversal, TOCTOU, and archive extraction risks. This needs an explicit trust model decision before code changes. (by @assistant)
