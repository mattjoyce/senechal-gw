# Changelog

## 2026-03-07
- Discord notify plugin now supports an incoming webhook, adds a poll command with configurable poll_message, and includes a default_message fallback plus a User-Agent fix.
- JSON Schemas for config, plugins, and pipelines YAML are added, with multi-file-merge support and validation for max_workers and plugin parallelism.
- Repo sync and discovery are hardened to prefer SSH remotes, use per-repo dedupe keys for concurrency, and exclude forks, with RO_GITHUB_TOKEN used for repo sync.
- Weekly changelog generation and auto-derived versioning are added via scripts/version.sh and Makefile.
- Testing and reliability improvements include stress-test suites, API and webhook mocks, and fixes to lint and webhook fixtures.

## [Unreleased] — Weekly Development Summary (Last 7 Days)

This entry captures the most significant changes from the past week of development. It is intentionally expansive so that it remains useful as older Git history scrolls out of view.

### Highlights
- Introduced **YouTube playlist ingestion** with an end-to-end transcript → Fabric → markdown workflow for Astro summaries.
- Implemented **CLI support for plugin list/run**, enabling direct API-backed invocation.
- Delivered **skills/capability discovery improvements** for AI operators and external tooling.
- Shipped **scheduler upgrades** (command-based schedules, dedupe cadence fixes, next-run persistence).
- Completed **RC1 cleanup** with removal of legacy config and auth surfaces.

### Added
- **youtube_playlist plugin** for polling YouTube playlists and emitting dedupe-safe events.
- **Plugin CLI actions**:
  - `ductile plugin list` (API-backed `/plugins`)
  - `ductile plugin run` (API-backed `/plugin/{name}/{command}`)
- **sys_exec plugin** for safe shell execution with env-only payload propagation.
- **file_watch / folder_watch plugins** with optional per-file events and dedupe propagation.
- **Skills registry enhancements**:
  - Expanded `/skills` index (pipelines + plugins)
  - AI-native skills manifest format improvements
- **Plugin instance aliasing** to allow per-instance configuration and reuse.
- **TUI watch improvements** with metadata header and richer detail panels.

### Changed
- **Scheduler**:
  - Command-based schedules (beyond implicit `poll` only).
  - Persisted `next_run` timestamps and improved countdown visibility in TUI.
  - Dedupe cadence fixes for frequent schedules.
  - Startup validation for scheduled commands.
- **Config and discovery**:
  - Plugin discovery moved earlier in startup for better preflight diagnostics.
  - Config logging now includes discovered/configured/enabled plugin counts.
- **Docs/identity**:
  - README/identity refreshed and aligned with integration-engine focus.
  - Host-local deployment guide added.
  - Webhooks and cookbook documentation expanded.

### Fixed
- YouTube playlist prompt formatting crash (metadata braces in prompt templates).
- Token loading and webhook token resolution in include mode.
- Multiple manifest command metadata omissions (command type fields added).

### Security / Hardening
- Symlink policy enforcement for config and plugins.
- Queue list hardening with safer where-clause builder.
- Secret redaction and reduced permissions for sensitive paths.
- gosec cleanup: suppressions and fixes consolidated.

### Breaking / RC1 Cleanup
- **Removed legacy schedule field** (`schedule:`) in favor of `schedules:`.
- **Removed legacy API auth key** (`api.auth.api_key`), enforcing scoped tokens.
- **Removed legacy `/trigger` endpoint**.
- **Removed legacy config discovery fallback**.
- **Webhook secrets now require `secret_ref`** (inline `secret` removed).
- **Manifest command objects required**; missing command types now hard-fail.
- **Checksums v1 dropped**.
- **CLI backward-compat aliases removed**.

### Additional Notables
- Added default output directory support in `file_handler`.
- Auto-detect JS runtime for `yt-dlp` in `youtube_transcript`.
- Documentation cleanup: removed RFC/agent scaffolding and deprecated artifacts.

---

If you want this entry split into versioned releases (e.g., `1.0.0-rc.1`), say the word and I’ll restructure it.
