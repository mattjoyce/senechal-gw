---
audience: [2, 6]
form: meta
density: expert
verified: 2026-04-27
---

# Docs Rethink — Persona-Driven Blueprint

**Status:** Working draft
**Companion to:** `docs/AUDIENCES.md`, `docs/DOCS_RUBRIC.md`
**Not:** a drift audit (see `docs/DOCS_AUDIT.md`), and not the new
documentation itself. This document is the *plan* for what the docs should
become.

---

## 1. Purpose

`AUDIENCES.md` defines eight reader cells across three axes
(Agent ↔ Human, Coder ↔ Operator, Learner ↔ Expert) and marks each cell's
current coverage. This document takes those cells as the design constraint
and asks, for each one:

1. What does this persona actually need to do?
2. What surface do they hit today, and how does it fail them?
3. What should they hit, in what form, at what density?
4. What concretely changes — layout, content, or affordance — to get there?

The aim is not to reorganise `docs/` for its own sake. The aim is to make
every cell resolve to a surface that fits its form, density, and domain. If
that means a doc moves, fine; if it means new content; fine; if it means a
new affordance (`ductile init`, a tagged teaching plugin, a runbook index),
that is a real change tracked as a `bd` card, not a docs edit.

This document is **aspirational**. It is reviewed and revised as a unit
before any file moves. Stage-by-stage migration is sequenced in §6.

---

## 2. Design commitments (carried over from AUDIENCES)

These are not re-derived here; they are the constraints the rethink must
honour.

- **Form follows axis.** Agent surfaces are machine-actionable; human
  surfaces are prose. Same facts, different projections.
- **Density does not mix.** Tutorial files and reference files are separate
  files. A page that tries to be both serves neither.
- **Coder ↔ Operator maps to `internal/` ↔ `~/.config/`.** A doc whose
  primary reader is changing Ductile sits in a different lane from a doc
  whose primary reader is running it.
- **Agent surfaces are first-class.** `schemas/`, `skills/`, OpenAPI are
  not a `docs/` subfolder; they are co-equal entry points.
- **Names are part of the contract.** No silent renames of canonical files
  (`ARCHITECTURE.md`, `PIPELINES.md`, `CONFIG_REFERENCE.md`); renames are a
  real change with a redirect plan.
- **Ephemera is not canon.** Working notes, RCAs, sprint logs, slides, and
  benchmark write-ups do not live next to reference docs.

---

## 3. Per-cell aspiration

Each subsection follows the same shape: **need → today → aspire → moves**.
Cells are numbered as in `AUDIENCES.md`.

### Cell 1 — H · C · L — first-time contributor / first-time plugin author

- **Need.** Thirty minutes from `git clone` to a working plugin on their
  machine. Acquire vocabulary by doing, not by reading 800-line specs.
- **Today.** `README.md` and `GETTING_STARTED.md` are install-flavoured;
  `PLUGIN_DEVELOPMENT.md` is reference-dense. The path from "I want to add
  a plugin" to "I have one running" is not signposted, and idioms now live
  in `AGENTS.md` which is not a learner doc.
- **Aspire.** A single landing page, *Build your first plugin*, that walks
  from clone → run → modify → observe in one canonical example, with
  vocabulary terms inline-linked to `GLOSSARY.md` on first use. The
  reference (`PLUGIN_DEVELOPMENT.md`) is what they graduate to, not what
  they start with.
- **Moves.**
  - **Create** `docs/learn/first-plugin.md` (tutorial, density: low).
  - **Tag** one bundled plugin (likely `plugins/echo`) as the canonical
    teaching example, referenced from the tutorial and from cell 5.
  - **Split** `PLUGIN_DEVELOPMENT.md` so reference content is reference
    only; tutorial-flavoured paragraphs migrate into the new learner page.
  - **Cross-link** vocabulary terms on first use in every learner page.

### Cell 2 — H · C · E — maintainer / experienced plugin author

- **Need.** Design decisions, invariants, and trade-offs in one click, so a
  change to the queue, router, or protocol does not break a constraint they
  did not know existed.
- **Today.** Served. `AGENTS.md` carries vocabulary, design grounding, and
  non-negotiables; `ARCHITECTURE.md` is the steady-state reference.
- **Aspire.** Same, plus per-package design notes for load-bearing packages
  (`queue`, `router`, `dispatch`, `state`, `protocol`) so a maintainer
  changing one package finds its design rationale beside its code.
- **Moves.**
  - **Create** `internal/<pkg>/DESIGN.md` for the load-bearing packages.
    Tracked as one card per package, not one mega-card.
  - **Keep** `ARCHITECTURE.md` as the system-level reference; package
    notes link up to it, not the other way around.

### Cell 3 — H · O · L — evaluator / first-time installer

- **Need.** Five minutes from "I just heard about this" to a live event
  flow on their machine, before reading prose.
- **Today.** `README.md` sells; `GETTING_STARTED.md` installs; the root
  `config.yaml` is the implicit welcome-mat and is **known stale**
  (see `DOCS_AUDIT.md`). The watch TUI was the demo affordance and is
  currently being torn out (`ductile-hickey-tui-rip-and-rewrite`).
- **Aspire.** `ductile init` produces a validated minimal config plus a
  curated `examples/` library. The README's "30 seconds" promise is backed
  by exactly one runnable path, not a copy-paste recipe that drifts.
- **Moves.**
  - **Affordance:** `ductile init` (tracked as a card; *not* a doc edit).
  - **Affordance:** `examples/` library, fixture-driven, tested.
  - **Rewrite** `GETTING_STARTED.md` around the `init` flow once it lands.
  - **Retire** the root `config.yaml` as a teaching artefact; it becomes a
    runtime default at most.
  - **Status:** deferred to Phase 3 in AUDIENCES; this rethink does not
    unblock it, only names the unblock.

### Cell 4 — H · O · E — veteran operator in production

- **Need.** Symptom-organised runbooks. *"Orphaned jobs after crash"*,
  *"Integrity check failing"*, *"Plugin in crash loop"*, *"Disk full"*.
  Recovery without reading source.
- **Today.** `OPERATOR_GUIDE.md` and `DEPLOYMENT.md` exist but are
  feature-organised, not symptom-organised. `PLUGIN_DIAGNOSTICS.md` is the
  closest thing to a runbook and is narrow. `reload_rca.md` is one
  incident, not a pattern.
- **Aspire.** A `docs/runbooks/` directory, one file per symptom, each with
  a fixed shape: *Symptom → How to confirm → Likely causes → Recovery
  steps → Prevention*. Indexed by symptom, not by feature.
- **Moves.**
  - **Create** `docs/runbooks/` with an index page and an initial set:
    `orphaned-jobs.md`, `integrity-failure.md`, `plugin-crash-loop.md`,
    `disk-full.md`, `config-reload-failed.md`, `webhook-hmac-rejected.md`.
  - **Delete** `reload_rca.md` and any sibling incident write-ups as
    legacy debris (resolved §7.3); runbooks are written fresh from
    current operational reality.
  - **Keep** `OPERATOR_GUIDE.md` as the steady-state operator reference;
    runbooks handle failure modes.
  - **Status:** content gap. No reorganisation produces this; it is
    written.

### Cell 5 — A · C · L — cold-start agent generating a plugin

- **Need.** Discover the plugin contract from machine-readable schemas and
  one canonical example, without hallucinating field names.
- **Today.** Schemas exist in `schemas/` but are not advertised; no plugin
  is explicitly tagged as the teaching example.
- **Aspire.** `schemas/` is a top-level published surface with a
  `schemas/README.md` index. One bundled plugin is explicitly tagged
  (manifest field or repo-level marker) as the canonical teaching example,
  cross-referenced from cell 1 and from the skill.
- **Moves.**
  - **Create** `schemas/README.md` (index of the eight schemas, stable
    URLs, version policy).
  - **Tag** `plugins/echo` (or whichever) as the teaching example via a
    convention agreed in `PLUGIN_DEVELOPMENT.md` and the skill.
  - **Advertise** schema paths from `skills/ductile/SKILL.md`.

### Cell 6 — A · C · E — in-repo coding agent

- **Need.** Style, safety, idioms, vocabulary in one file at a predictable
  path; a workflow it can drive (`bd`).
- **Today.** Served. `AGENTS.md` is the contract, doc-smoke lint covers
  it, `bd` workflow is in place.
- **Aspire.** Same, plus the per-package design notes from cell 2 (which
  agents read for the same reasons humans do).
- **Moves.** None unique to this cell; rides on cell 2's package notes.

### Cell 7 — A · O · L — agent doing first-time setup

- **Need.** A deterministic way to generate a valid initial config and
  verify it. Never produce config that fails `ductile config check`.
- **Today.** Gap. Same root cause as cell 3.
- **Aspire.** `ductile init` (shared with cell 3) plus
  `schemas/config.schema.json` advertised at a stable URL plus an
  agent-readable `examples/` index.
- **Moves.** Same as cell 3, plus surfacing `ductile init` and schema URLs
  through `skills/ductile/SKILL.md`.
- **Status:** deferred to Phase 3 with cell 3.

### Cell 8 — A · O · E — agent operating live Ductile

- **Need.** Every operation discoverable through `/skills` and OpenAPI;
  agent-readable recovery flows for live incidents.
- **Today.** `/skills` and OpenAPI exist; agent-readable runbooks do not.
- **Aspire.** The runbooks from cell 4 published in two forms: prose
  (`docs/runbooks/`) and structured (`/skills` recovery entries pointing at
  the same recovery steps). Same content, two projections.
- **Moves.**
  - **Add** a recovery section to `skills/ductile/SKILL.md` that mirrors
    the `docs/runbooks/` index entries by ID, so an agent can pick a
    runbook by symptom and execute its steps.
  - **Convention:** every runbook has a stable ID; the skill references
    runbooks by ID, not URL.

---

## 4. Inventory: current `docs/` against cells

Disposition is one of: **keep** (canonical, lives in target taxonomy),
**split** (separate tutorial and reference content), **merge** (fold into
another canonical doc), **move** (out of `docs/`; ephemera or working
note), **retire** (delete or archive), **create** (does not exist yet).

| File | Primary cell(s) | Disposition | Notes |
|---|---|---|---|
| `README.md` (root) | 3, 1 | keep | Tightened around `ductile init` once it lands. |
| `AGENTS.md` (root) | 6, 2 | keep | Canonical contract. |
| `CONTRIBUTING.md` (root) | 1, 6 | keep | Mechanics; cross-link to cell-1 tutorial when written. |
| `docs/AUDIENCES.md` | meta | keep | The taxonomy. |
| `docs/ARCHITECTURE.md` | 2 | keep | System reference. |
| `docs/GETTING_STARTED.md` | 3, 1 | split | Evaluator path → cell 3 (rewrite around `init`); contributor first-plugin path → new cell-1 tutorial. |
| `docs/PLUGIN_DEVELOPMENT.md` | 2, 1 | split | Reference stays; tutorial paragraphs migrate to cell-1 tutorial. |
| `docs/PIPELINES.md` | 2 | keep | Reference. |
| `docs/ROUTING_SPEC.md` | 2 | keep | Reference. |
| `docs/CONFIG_REFERENCE.md` | 2, 4 | keep | Reference. |
| `docs/API_REFERENCE.md` | 2, 8 | keep | Reference; resolve drift items in `DOCS_AUDIT.md`. |
| `docs/DATABASE.md` | 2, 4 | keep | Reference. |
| `docs/SCHEDULER.md` | 2 | keep | Reference. |
| `docs/WEBHOOKS.md` | 2, 4 | keep | Reference. |
| `docs/PLUGIN_FACTS.md` | 2 | keep | Reference. |
| `docs/PLUGIN_DIAGNOSTICS.md` | 4 | merge | Best parts become symptom runbooks; remainder folds into `OPERATOR_GUIDE.md`. |
| `docs/OPERATOR_GUIDE.md` | 4 | keep | Steady-state operator reference; failure modes leave for `runbooks/`. |
| `docs/DEPLOYMENT.md` | 4, 3 | keep | Reference. |
| `docs/MACOS_INSTALLATION.md` | 3 | keep | Platform-specific install. |
| `docs/COOKBOOK.md` | 1, 4 | split | Tutorial-shaped recipes → `learn/`; operational patterns → `runbooks/` or `OPERATOR_GUIDE.md`. |
| `docs/10_IDIOMS_OF_DUCTILE.md` | 2, 1 | keep | Idioms; cross-linked from cell-1 tutorial. |
| `docs/GLOSSARY.md` | all | keep | Single source of vocabulary; first-use links land here. |
| `docs/TESTING.md` | 6, 2 | keep | Contributor reference. |
| `docs/VERSIONING.md` | 2, 4 | keep | Reference; resolve drift items in `DOCS_AUDIT.md`. |
| `docs/CLI_DESIGN_PRINCIPLES.md` | 2 | keep | Internal design rationale; candidate to move to `internal/cli/DESIGN.md` when package notes land. |
| `docs/YAML_TIPS.md` | 1, 3 | keep | Learner aid; cross-link from cell-1 tutorial. |
| `docs/DUCTILE_SKILLS_SCHEMA_V1.md` | 8 | keep | Agent-surface reference. |
| `docs/DOCS_AUDIT.md` | meta | keep | Drift audit; remains until items are resolved, then archived. |
| `docs/DOCS_RETHINK.md` | meta | keep | This document. |
| `docs/DUCTILE_NEW_DEVELOPER_SLIDES.html` | 1 | move | Slides; out of `docs/` (e.g. `assets/talks/` or repo wiki). |
| `docs/Orchestration_Primitives - proposal.md` | meta | move | Working note; out of `docs/`. |
| `docs/CONCURRENCY_BENCH_2026-03-04.md` | 2 | move | Bench write-up; out of `docs/` (e.g. `notes/` or beads attachment). |
| `docs/reload_rca.md` | 4 | retire | Legacy incident debris; deleted, not promoted (see §7.3). |
| `docs/ZK_2026-03-04_concurrency-rollout.md` | meta | move | Sprint log. |
| `docs/ZK_2026-03-05_docs-refresh.md` | meta | move | Sprint log. |

**Net effect on `docs/`:**

- 1 file retired (`reload_rca.md`, legacy debris). Five files leave
  `docs/` as ephemera into `notes/`; none of those are lost.
- Three files split (`GETTING_STARTED`, `PLUGIN_DEVELOPMENT`, `COOKBOOK`).
- Two new directories: `docs/learn/`, `docs/runbooks/`.
- One file merges (`PLUGIN_DIAGNOSTICS` → runbooks + operator guide).
- ~6 new files created across cells 1, 4, 5, plus per-package `DESIGN.md`.

---

## 5. Where ephemera goes

`docs/` is canon. Ephemera (RCAs, sprint logs, bench write-ups, proposals,
slides) needs a home that is *not* `docs/` and *not* lost.

Options, in preference order:

1. **`notes/` at repo root** — versioned, dated, explicitly non-canon.
   Lowest friction; visible to maintainers; never confused with reference.
2. **Beads attachments** — durable beside the work that produced them.
   Best for RCAs and bench notes tied to a card.
3. **Obsidian vault** — exploratory work that has no permanent home in the
   repo (e.g. `ductile-hickey-tui-rip-and-rewrite`).

Recommendation: **`notes/`** for slides, proposals, sprint logs, bench
write-ups, RCAs whose pattern has been promoted; **beads** for anything
still actively producing decisions; **Obsidian** for design exploration
that has not yet landed in the repo.

---

## 6. Sequenced plan

The rethink is not one PR. Each phase is independently mergeable.

### Phase 0 — agree on this document
- Review and revise `docs/DOCS_RETHINK.md`.
- Open a `bd` card for the overall effort; sub-cards per phase.

### Phase 1 — clear the deck (low risk, high signal)
- Move ephemera out of `docs/` (five files → `notes/`); retire
  `reload_rca.md`.
- Resolve outstanding `DOCS_AUDIT.md` items so reference docs stop lying.
- Add `schemas/README.md` (repo-path index; see §7.2).
- **Build** the canonical teaching plugins `watchfile` and `notify` plus
  the pipeline that wires them, under `plugins/` (see §7.1). Add a
  *teaching plugin* convention and tag this pair as the canonical
  example.

### Phase 2 — separate density (medium risk)
- Create `docs/learn/` with cell-1 tutorial *Build your first plugin*.
- Split `GETTING_STARTED.md`, `PLUGIN_DEVELOPMENT.md`, `COOKBOOK.md`.
- Cross-link `GLOSSARY.md` from every learner page first-use.

### Phase 3 — close the operator gap (content work)
- Create `docs/runbooks/` with index and the initial six runbooks.
- Mirror runbook IDs in `skills/ductile/SKILL.md` (cell 8).
- Promote the `reload_rca.md` pattern; archive the RCA.

### Phase 4 — affordances (deferred from AUDIENCES)
- `ductile init` (cells 3 and 7).
- Curated `examples/` library.
- Rewrite cell-3 `GETTING_STARTED.md` around `init`.

### Phase 5 — package design notes (cell 2 / cell 6)
- **First card sets the template** for `internal/<pkg>/DESIGN.md`
  (shape, headings, depth, link discipline up to `ARCHITECTURE.md`); see
  §7.4. No further package notes are written until the template is
  reviewed.
- Then one card per load-bearing package: `queue`, `router`, `dispatch`,
  `state`, `protocol`. Each lands an `internal/<pkg>/DESIGN.md` matching
  the template.
- Move `CLI_DESIGN_PRINCIPLES.md` into `internal/cli/DESIGN.md` if and
  when that package note is written.

Phases 1 and 2 are pure docs work and can run in parallel with feature
work. Phases 3 and 4 are content/affordance work and are sized as feature
cards. Phase 5 is opportunistic — written when the package is being
changed, not in a campaign.

---

## 7. Resolved decisions

The questions raised in earlier drafts are resolved as follows. They are
recorded here, not in §3, so the rationale survives if a decision is
revisited.

1. **Teaching plugin identity → `watchfile` + `notify` (two plugins, one
   pipeline).** Neither exists today; both are in scope to build as part
   of Phase 1. `watchfile` polls a local file's mtime and emits a
   `plugin_fact` when it changes, forwarding path and mtime as baggage.
   `notify` consumes the baggage and appends a line to a log file. The
   pair exercises facts (dedup by mtime), baggage propagation, a two-step
   pipeline, and a schedule trigger, with no external network dependency
   so the tutorial cannot fail for environmental reasons. Rejected
   alternatives: `plugins/echo` alone (does not exercise facts or
   pipelines); a single `counter` plugin (contrived); a webhook-based
   example (skips facts, requires HMAC setup before the learner has
   vocabulary for it).
2. **Schema discoverability → repo paths only.** `schemas/*.json` is
   referenced by repo path. No stable HTTP hosting commitment. The skill
   advertises repo-relative paths; agents resolve them through whatever
   checkout they are working against.
3. **Runbook authoring source → write from current operational reality;
   delete legacy RCA docs.** `reload_rca.md` and any sibling incident
   write-ups are removed outright as legacy debris, not promoted into
   runbooks. The initial runbooks are written from current operator
   knowledge and beads history; nothing is mined from `docs/`.
4. **`internal/<pkg>/DESIGN.md` precedent → no precedent exists; the
   first one sets the template.** Confirmed: `find internal -iname
   DESIGN.md` returns nothing today. Phase 5 therefore opens with a
   deliberate template-setting card (shape, headings, depth, link
   discipline up to `ARCHITECTURE.md`) before any package note is
   written. Subsequent packages copy the template; deviations are real
   design decisions, not drift.
5. **Skill-runbook coupling → reference by stable ID.** Runbook prose
   lives in `docs/runbooks/<id>.md`. The skill carries the index of IDs
   and the symptoms they address; it does not embed prose. Single source
   of truth, two projections.

---

## 8. Out of scope

- Rewriting reference documents that are already correct and serve cell 2.
- Tooling changes beyond `ductile init` and the `examples/` library.
- Marketing-shaped revisions of `README.md` beyond aligning it with the
  cell-3 evaluator path.
- Internationalisation, theming, search, or doc-site generators. Plain
  Markdown remains the substrate.

---

## 9. Maintenance

This document is owned by the docs taxonomy, not by any single phase. When
a phase merges, update the per-cell *Today* and *Status* in
`AUDIENCES.md`, then update §3 and §6 here. When a cell's aspiration
changes, the rationale change goes here first; only then do moves follow.

---

## 10. Maintenance rubric

This blueprint defines *what* the docs should become; on its own it does
not keep them that way. The maintenance rubric — `docs/DOCS_RUBRIC.md` —
carries the standard every canon document is held to: front-matter,
density rules, linking discipline, coupling and freshness, runbook shape,
and the lint that enforces them.

The rubric is anchored on a single principle: **reader mode governs
layout.** Reference rewards coherence (looking up). Runbooks reward
isolation (dispatching under stress). Tutorials reward narrative
(learning). Layout follows mode, not preference.

**Phase 1 of §6 picks up two rubric obligations:**

1. Add the required front-matter to every canon document.
2. Land the rubric lint (`internal/docs/rubric_test.go`) at
   presence-only strictness (front-matter present, fields valid, links
   resolve). Stricter rules (density legality, runbook ID uniqueness,
   freshness windows) activate in subsequent phases as the relevant
   surfaces appear.

The rubric and this blueprint are co-maintained. A change to one without
a corresponding update to the other is a rubric violation in itself.
