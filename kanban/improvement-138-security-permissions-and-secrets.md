---
id: 138
status: todo
priority: Normal
blocked_by: []
tags: [improvement, security, permissions, config]
---

# Security: Permissions Baseline + Secrets Exposure Audit

## Problem
Gosec flags multiple cases where file/dir permissions are wider than recommended, plus a case where a `Secret` field is marshaled to JSON. These are likely policy decisions rather than bugs, but we should codify the expected defaults.

## Evidence (gosec.json excerpts)
```json
{
  "rule_id": "G301",
  "details": "Expect directory permissions to be 0750 or less",
  "file": "internal/workspace/fs_manager.go",
  "line": "45"
}
{
  "rule_id": "G302",
  "details": "Expect file permissions to be 0600 or less",
  "file": "internal/lock/pidlock.go",
  "line": "27"
}
{
  "rule_id": "G306",
  "details": "Expect WriteFile permissions to be 0600 or less",
  "file": "internal/config/access.go",
  "line": "235"
}
{
  "rule_id": "G117",
  "details": "Marshaled struct field \"Secret\" matches secret pattern",
  "file": "cmd/ductile/config_manage.go",
  "line": "1704"
}
```

## Proposed Direction
- Decide baseline permissions for config files, lock files, and workspace dirs (likely 0600/0700 unless a shared group is intended).
- Ensure CLI output does not print secrets unless explicitly requested (masking or redaction by default).

## Acceptance Criteria
- [ ] Document and enforce default permissions for config/lock/workspace artifacts.
- [ ] Update any `WriteFile/OpenFile/MkdirAll` calls that should be tighter.
- [ ] Audit JSON output for secrets and add masking or require explicit `--show-secrets` if needed.

## Notes (assistant’s take)
This isn’t design-breaking. It’s a baseline policy decision: single-user operator vs shared runtime. I’d default to least‑privilege (0600/0700) unless there’s a documented reason not to.

## Narrative
- 2026-02-28: Created after gosec flagged permissions and secret marshaling; requires policy alignment rather than major design changes. (by @assistant)
