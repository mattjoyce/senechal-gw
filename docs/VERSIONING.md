# Versioning and Build Metadata

This document defines how `ductile` versions are assigned and how build metadata is injected into binaries.

## Versioning Policy

Ductile uses **auto-derived versioning** — no manual version bumps, no release tags required.

Format: `v0.<commit-count>-<short-hash>`

Examples:
- `v0.381-ca42b56`
- `v0.382-f1a3c9d`

The commit count is monotonically increasing. The short hash uniquely identifies the exact source state. Together they give a human-readable, sortable, unambiguous version without any ceremony.

## CLI Version Output

Both commands are supported:

- `ductile version`
- `ductile --version`

Machine-readable output:

- `ductile version --json`

Version output includes:

- version string (`v0.<count>-<hash>`)
- git commit (short SHA)
- build time in UTC (RFC3339)

## Deriving the Version

The canonical source is `scripts/version.sh`:

```sh
#!/bin/sh
echo "v0.$(git rev-list --count HEAD)-$(git rev-parse --short HEAD)"
```

This script runs identically in local builds, Docker/Unraid, and any future CI. No state file, no manual step.

## Building Locally

```bash
make install
```

This builds with full ldflags and restarts `ductile-local` via systemd. See `Makefile` for details.

## Build Metadata Injection

Three ldflags variables are injected at build time:

| Variable | Value |
|---|---|
| `main.version` | Output of `scripts/version.sh` |
| `main.gitCommit` | `git rev-parse --short HEAD` |
| `main.buildDate` | UTC timestamp at build time |

## Dockerfile (Unraid)

The production Dockerfile calls `scripts/version.sh` during the build stage — same script, same version, no divergence.
