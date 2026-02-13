# Versioning and Build Metadata

This document defines how `ductile` versions are assigned and how build metadata is injected into binaries.

## Versioning Policy

Ductile follows Semantic Versioning (`MAJOR.MINOR.PATCH`):

- `MAJOR`: Breaking CLI/config/protocol changes.
- `MINOR`: Backward-compatible features.
- `PATCH`: Backward-compatible fixes.

Pre-release builds use SemVer pre-release tags, for example:

- `1.4.0-rc.1`
- `1.4.0-beta.2`

## CLI Version Output

Both commands are supported:

- `ductile version`
- `ductile --version`

Machine-readable output is available with:

- `ductile version --json`
- `ductile --version --json`

Version output includes:

- semantic version
- git commit (short SHA)
- build time in UTC (RFC3339)

## Build Metadata Injection

The CLI reads these linker-injected variables:

- `main.version`
- `main.gitCommit`
- `main.buildDate`

Release build example:

```bash
VERSION=1.2.0
COMMIT=$(git rev-parse --short=12 HEAD)
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

go build \
  -ldflags "-X main.version=${VERSION} -X main.gitCommit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
  -o ductile \
  ./cmd/ductile
```

## Local/Dev Build Behavior

For local builds without explicit ldflags:

- `version` defaults to `0.1.0-dev`
- commit/build time are discovered from Go VCS build info when available
- if VCS metadata is unavailable, commit/build time are reported as `unknown`

## Release Tagging Process

1. Update release notes/changelog.
2. Create and push an annotated SemVer tag (for example `v1.2.0`).
3. Build with ldflags shown above.
4. Verify:
   - `./ductile --version`
   - `./ductile --version --json`
