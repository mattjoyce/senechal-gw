---
id: 32
status: done
priority: Normal
blocked_by: []
tags: [reliability, user-experience, diagnostics]
---

# Detect and warn when database path is on network filesystem

When SQLite initialization fails on SMB/NFS/CIFS filesystems, the error is cryptic (`libc_darwin.go:224:Xsrandomdev: TODOTODO`). Users cannot diagnose the root cause.

## Problem

`modernc.org/sqlite` (pure-Go driver) fails on network filesystems with implementation-specific errors that don't indicate the actual issue. Example:
- Working: `/Users/user/senechal.db` on local APFS
- Failing: `/Volumes/Projects/senechal.db` on SMB share
- Same binary, same config, different filesystem = cryptic failure

## Acceptance Criteria

- Before opening SQLite connection, check if database path resolves to a network filesystem (SMB, NFS, CIFS, AFP)
- If network filesystem detected, exit immediately with clear error message
- Error message includes:
  - Detected filesystem type (e.g., "smbfs")
  - Explanation that SQLite requires local filesystem
  - Actionable remediation: use `--db /path/to/local/file.db` flag or move working directory to local disk
- Detection works on Darwin (macOS) and Linux
- Add test case with mock filesystem checks

## Implementation Notes

On macOS, use `statfs()` syscall to get filesystem type. On Linux, similar approach with `/proc/mounts` or `statfs.f_type`.

Common network filesystem identifiers:
- macOS: `smbfs`, `nfs`, `afpfs`, `webdav`
- Linux: `nfs`, `cifs`, `smbfs`

## Narrative

- 2026-02-09: Discovered when running from `/Volumes/Projects` (SMB share) vs `~/Projects` (local APFS). Same repo:branch, different outcomes. Added to improve first-run diagnostics. (by @claude)
- 2026-02-11: Added startup preflight in `internal/storage` to detect network filesystems before SQLite open (Darwin via `statfs().Fstypename`, Linux via `statfs().Type` magic mapping) and fail fast with actionable remediation text including local `state.path` / `--db` guidance. Added mock-driven tests for local pass, network reject, and nearest-existing-path resolution; full `go test ./...` passes. (by @codex)
