---
id: 57
status: done
priority: High
blocked_by: []
tags: [config, cli, checksums, ux]
---

# Add Verbose Progress Output to `config hash-update`

Add `-v` / `--verbose` and `--dry-run` to `senechal-gw config hash-update` so operators can see progress for each processed config scope file and its computed hash before writing changes.

## Job Story

When I run `config hash-update` in include-based config mode, I want per-file progress and hash output with an optional dry run, so I can verify exactly what would be re-hashed and where before writing `.checksums`.

## Acceptance Criteria

- `senechal-gw config hash-update` accepts `-v` and `--verbose` (equivalent behavior).
- `senechal-gw config hash-update` accepts `--dry-run` to preview without writing `.checksums`.
- In verbose mode, command prints progress per target directory and per scope file (`tokens.yaml`, `webhooks.yaml`) encountered.
- Verbose output includes computed hash values for files that exist.
- Verbose output clearly indicates skipped optional files that are absent.
- In `--dry-run` mode, output clearly indicates no files were written.
- Non-verbose output remains concise and backward compatible.
- Existing checksum generation behavior is unchanged when `--dry-run` is not set (only output/flag surface changes).
- Unit tests cover verbose flag parsing and representative output behavior.

## Narrative
- 2026-02-12: Card created to improve checksum regeneration visibility now that hash-update supports root config + includes. (by @assistant)
- 2026-02-12: Implemented `-v`/`--verbose` and `--dry-run` for `config hash-update` with per-file hash/skip output, plus unit tests for config-layer reporting and CLI flag behavior. (by @assistant)
