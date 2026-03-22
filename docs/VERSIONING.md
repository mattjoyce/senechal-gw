# Versioning and Build Metadata

This document defines how `ductile` versions are assigned and how build metadata is injected into binaries.

## Versioning Policy

Ductile uses **Semantic Versioning (SemVer)** for releases and release candidates.

Format: `v<major>.<minor>.<patch>[-<pre-release>]`

Examples:
- `v1.0.0-rc.1`
- `v1.0.1`

Development builds may include additional metadata like commit counts or short hashes if derived via the build scripts.

## CLI Version Output

Both commands are supported:

- `ductile version`
- `ductile --version`

Machine-readable output:

- `ductile version --json`

Version output includes:

- version string (`v1.0.0-rc.1`)
- git commit (short SHA)
- build time in UTC (RFC3339)

## Deriving the Version

Versions are typically set in `cmd/ductile/main.go`. The build process can override these using ldflags.

## Building Locally

```bash
make install
```

This builds with full ldflags and restarts `ductile-local` via systemd. See `Makefile` for details.

## Build Metadata Injection

Three ldflags variables are injected at build time:

| Variable | Value |
|---|---|
| `main.version` | The target version string |
| `main.gitCommit` | `git rev-parse --short HEAD` |
| `main.buildDate` | UTC timestamp at build time |

## Dockerfile (Unraid)

The production Dockerfile calls `scripts/version.sh` during the build stage — same script, same version, no divergence.
