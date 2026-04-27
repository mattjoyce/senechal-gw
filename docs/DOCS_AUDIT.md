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
**Resolved 2026-04-27 in `docs/cardfc9.1-phase1-clear-deck-frontmatter`:** DEPLOYMENT.md already shows `state: { path: ./data/ductile.db }`; verified during Phase 1 sweep. No further fix required.

---

## Moderate Issues

### 6. API_REFERENCE.md missing endpoints

**Undocumented endpoints:**
- `GET /events` — SSE event stream (requires `events:ro`)
- `GET /scheduler/jobs` — scheduler job status (requires `jobs:ro`)
- `POST /system/reload` — config reload (requires `system:rw`)
- `GET /` — root discovery index (unauthenticated)
**Resolved 2026-04-27 in `docs/cardfc9.1-phase1-clear-deck-frontmatter`:** Root discovery (§0) and `POST /system/reload` (§10) were already documented; added §13 (`GET /events`) and §14 (`GET /scheduler/jobs`).

### 7. API_REFERENCE.md section numbering

- Section 4 is missing (jumps 3 → 5)
- Section 7 is duplicated (Health and OpenAPI Discovery both numbered 7)
**Resolved 2026-04-27 in `docs/cardfc9.1-phase1-clear-deck-frontmatter`:** Renumbered Jobs List §5→§4, Job Logs §6→§5, System Health §7→§6, OpenAPI Discovery §7→§7 (no longer duplicated). Trailing sections (§8–§12) unchanged.

### 8. `system:rw` scope undocumented

**Reality:** `system:rw` is enforced for `/system/reload` in server.go and validated by doctor.go.
**Docs:** CONFIG_REFERENCE.md §5.2 lists only `plugin:*`, `jobs:*`, `events:*`, and `*`. No mention of `system:rw`.
**Resolved 2026-04-27 in `docs/cardfc9.1-phase1-clear-deck-frontmatter`:** CONFIG_REFERENCE.md §5.2 now lists `system:rw` and per-plugin scope syntax.

### 9. OPERATOR_GUIDE.md reload command marked "planned"

**Doc:** Shows `# This command is planned for a future release` above `./ductile system reload`.
**Reality:** `system reload` is fully implemented (SIGHUP + API fallback). Works.
**Resolved 2026-04-27 in `docs/cardfc9.1-phase1-clear-deck-frontmatter`:** Verified during Phase 1 sweep — OPERATOR_GUIDE.md no longer carries the "planned" comment; no further fix required.

### 10. OPERATOR_GUIDE.md scope example uses `plugin:echo:rw`

**Doc:** "...checkbox list of available scopes (e.g., `jobs:ro`, `plugin:echo:rw`)."
**Reality:** Scope format for per-plugin access is `echo:rw` (plugin name as resource). `plugin:echo:rw` is not a valid scope — `plugin` is a global resource (`plugin:ro`, `plugin:rw`), and per-plugin scopes use the plugin name directly.
**Resolved 2026-04-27 in `docs/cardfc9.1-phase1-clear-deck-frontmatter`:** Replaced example with `jobs:ro` (global) and `echo:rw` (per-plugin) with explanatory note.

### 11. GETTING_STARTED.md Go version is wrong

**Doc:** "requires version **1.25.4** or newer."
**Reality:** `go.mod` declares `go 1.26.0`.
**Resolved 2026-04-27 in `docs/cardfc9.1-phase1-clear-deck-frontmatter`:** Current `go.mod` declares `go 1.25.0`; updated GETTING_STARTED.md and the README badge to `1.25.0` to match present reality.

### 12. Healthz response in docs incomplete

**Doc (API_REFERENCE.md):** Shows `plugins_loaded`, `queue_depth` but omits `plugins_circuit_open`, `config_path`, `binary_path`, `version`.
**Reality:** HealthzResponse includes all of those fields.
**Resolved 2026-04-27 in `docs/cardfc9.1-phase1-clear-deck-frontmatter`:** `config_path`, `binary_path`, `version` were added previously; this sweep adds `plugins_circuit_open` and an explanatory line.

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
**Resolved 2026-04-27 in `docs/cardfc9.1-phase1-clear-deck-frontmatter`:** ARCHITECTURE.md example now uses canonical `jobs:ro`/`events:ro` plus per-plugin (`github-handler:rw`, `withings:ro`); CONFIG_REFERENCE.md §5.2 documents both global and per-plugin scope formats and notes current enforcement granularity.

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

## Resolution Status (2026-04-27)

The following pure-doc items were resolved in branch
`docs/cardfc9.1-phase1-clear-deck-frontmatter` as part of Phase 1 of
`DOCS_RETHINK.md`: §5, §6, §7, §8, §9, §10, §11, §12, §16.

Deferred to a separate code-agent card (require code or config
artefact changes, not just Markdown):

- §1 Dockerfile build does not call `scripts/version.sh`.
- §2 Dockerfile `CMD` uses `./ductile start` instead of `system start`.
- §3 Root `config.yaml` uses deprecated `schedule:` (singular).
- §4 Root `config.yaml` declares undeclared top-level
  `max_attempts` / `timeout` on plugins.
- §14 VERSIONING.md default version-string discrepancy with code
  fallback (`1.0.0-rc.1`).
- §15 `config/` example also uses deprecated `schedule:` syntax.
- §18 API scope enforcement granularity (code change implied if
  per-plugin enforcement is to match doc tone).

Items §13 and §17 were already confirmed OK in the original audit.
