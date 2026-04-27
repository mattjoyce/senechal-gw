---
audience: [2, 6]
form: meta
density: expert
verified: 2026-04-27
---

# Documentation vs Reality Audit

**Date:** 2026-03-03
**Branch:** `chore/docs-vs-reality-audit`

---

## Critical Issues

### 1. Dockerfile does NOT use `scripts/version.sh` (VERSIONING.md claims it does)

**Doc:** VERSIONING.md §Dockerfile says "The production Dockerfile calls `scripts/version.sh` during the build stage."
**Reality:** Dockerfile uses bare `go build -a -ldflags="-w -s"` — no version injection at all. Binaries built from Docker have version `1.0.0-rc.1`, commit `unknown`, build time `unknown`.
**Fix:** Add version.sh call to Dockerfile build stage ldflags.

### 2. Dockerfile CMD is `./ductile start` — invalid command

**Doc/Reality:** CLI requires `system start` (noun-verb). `start` is not a top-level command.
**Fix:** Change Dockerfile CMD to `["./ductile", "system", "start"]`.

### 3. Root `config.yaml` uses deprecated `schedule:` (singular)

**Reality:** Loader rejects `schedule:` with error: "schedule is no longer supported; use schedules[]". The root config.yaml for echo and youtube_playlist would fail validation.
**Fix:** Update config.yaml to use `schedules:` (plural array form).

### 4. Root `config.yaml` uses undeclared top-level `max_attempts` and `timeout` on plugins

**Reality:** `PluginConf` has no top-level `max_attempts` or `timeout` fields. These are silently ignored by the YAML parser. They belong under `retry.max_attempts` and `timeouts.poll` respectively.
**Fix:** Restructure config.yaml plugin entries to use `retry:` and `timeouts:` blocks.

### 5. DEPLOYMENT.md uses invalid config path `state.database.path`

**Doc:** Shows `state: { database: { path: ... } }`.
**Reality:** Config struct has `State.Path` (yaml: `state.path`) and `Database.Path` (yaml: `database.path`) as separate top-level aliases. `state.database.path` doesn't map to anything.
**Fix:** Change to `state: { path: ./data/ductile.db }` or `database: { path: ... }`.

---

## Moderate Issues

### 6. API_REFERENCE.md missing endpoints

**Undocumented endpoints:**
- `GET /events` — SSE event stream (requires `events:ro`)
- `GET /scheduler/jobs` — scheduler job status (requires `jobs:ro`)
- `POST /system/reload` — config reload (requires `system:rw`)
- `GET /` — root discovery index (unauthenticated)

### 7. API_REFERENCE.md section numbering

- Section 4 is missing (jumps 3 → 5)
- Section 7 is duplicated (Health and OpenAPI Discovery both numbered 7)

### 8. `system:rw` scope undocumented

**Reality:** `system:rw` is enforced for `/system/reload` in server.go and validated by doctor.go.
**Docs:** CONFIG_REFERENCE.md §5.2 lists only `plugin:*`, `jobs:*`, `events:*`, and `*`. No mention of `system:rw`.

### 9. OPERATOR_GUIDE.md reload command marked "planned"

**Doc:** Shows `# This command is planned for a future release` above `./ductile system reload`.
**Reality:** `system reload` is fully implemented (SIGHUP + API fallback). Works.

### 10. OPERATOR_GUIDE.md scope example uses `plugin:echo:rw`

**Doc:** "...checkbox list of available scopes (e.g., `jobs:ro`, `plugin:echo:rw`)."
**Reality:** Scope format for per-plugin access is `echo:rw` (plugin name as resource). `plugin:echo:rw` is not a valid scope — `plugin` is a global resource (`plugin:ro`, `plugin:rw`), and per-plugin scopes use the plugin name directly.

### 11. GETTING_STARTED.md Go version is wrong

**Doc:** "requires version **1.25.4** or newer."
**Reality:** `go.mod` declares `go 1.26.0`.

### 12. Healthz response in docs incomplete

**Doc (API_REFERENCE.md):** Shows `plugins_loaded`, `queue_depth` but omits `plugins_circuit_open`, `config_path`, `binary_path`, `version`.
**Reality:** HealthzResponse includes all of those fields.

---

## Minor Issues

### 13. VERSIONING.md `--version --json` discrepancy

**Doc:** Says `ductile --version --json` is supported.
**Reality:** `--version` delegates to `runVersion(args)` which does accept `--json`, so this works. ✅ (confirmed OK)

### 14. VERSIONING.md default version string

**Doc:** Says version defaults to auto-derived format.
**Reality:** Default (no ldflags) is `1.0.0-rc.1`, not `v0.<count>-<hash>`. The doc describes what happens *with* the Makefile, but the fallback doesn't match the doc's "Local/Dev Build Behavior" section (which was removed in the update but the code still has `1.0.0-rc.1`).

### 15. GETTING_STARTED.md references `config/` directory

**Doc:** Says "copy that folder to your config dir."
**Reality:** `config/` directory exists and has example files. ✅ OK, but the example config also uses deprecated `schedule:` syntax.

### 16. CONFIG_REFERENCE.md and ARCHITECTURE.md scope format inconsistency

**ARCHITECTURE.md** example scope file shows `"read:jobs"` and `"github-handler:rw"` (low-level + plugin-name format).
**CONFIG_REFERENCE.md** shows `"plugin:ro"`, `"jobs:ro"` (global resource format).
Both are valid per validation but serve different purposes — docs don't explain the distinction.

### 17. `--version --json` output key

**Doc (VERSIONING.md):** Says output includes "version string", "git commit", "build time".
**Reality:** JSON keys are `version`, `commit`, `build_time`. ✅ Matches.

### 18. API scope enforcement is global, not per-plugin

**Reality:** `handlePluginTrigger` checks `plugin:ro`/`plugin:rw` globally, not per-plugin scopes like `echo:ro`. Per-plugin scopes (`echo:ro`, `echo:rw`, `echo:allow:poll`) are valid in config/doctor validation but NOT enforced at the API middleware level.
**Doc implication:** Architecture docs suggest manifest-driven scopes influence authorization, but actual enforcement is coarse-grained.

---

## Summary

| Severity | Count |
|----------|-------|
| Critical | 5 |
| Moderate | 7 |
| Minor    | 6 |
