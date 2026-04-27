---
audience: [2, 6]
form: meta
density: expert
verified: 2026-04-27
---

# Audiences

Three orthogonal axes define who reads Ductile's documentation and uses its
surfaces. Eight cells. Each cell is a real reader, and every documentation
or software affordance can be evaluated against the cells it serves.

This document is **taxonomy-neutral**: it describes who the readers are, not
how `docs/` is organised today. The doc layout should serve these audiences;
when it stops doing so, the layout changes — these definitions do not.

---

## Axes

| Axis | Distinction | What it controls |
|---|---|---|
| **Agent ↔ Human** | Who is reading? | *Form*: machine-actionable schemas/skills vs narrative prose. |
| **Coder ↔ Operator** | Are they changing Ductile, or running it? | *Domain*: `internal/`-facing code surfaces vs `~/.config/`-facing runtime surfaces. |
| **Learner ↔ Expert** | Forming a mental model, or looking something up? | *Density*: tutorial + one example vs reference + invariants. |

The axes are independent. A persona is *not* a stereotype; it is the
intersection of three deliberate choices.

---

## The eight cells

| # | Persona | Axes | Landing surface today | Coverage |
|---|---|---|---|---|
| 1 | New contributor / first-time plugin author | H · C · L | `README.md`, `docs/GETTING_STARTED.md`, `docs/PLUGIN_DEVELOPMENT.md` | **partial** |
| 2 | Maintainer / experienced plugin author | H · C · E | `AGENTS.md`, `docs/ARCHITECTURE.md`, `docs/PIPELINES.md` | **served** |
| 3 | Evaluator / first-time installer | H · O · L | `README.md`, `docs/GETTING_STARTED.md`, `docs/MACOS_INSTALLATION.md` | **partial** |
| 4 | Veteran operator running Ductile in production | H · O · E | `docs/OPERATOR_GUIDE.md`, `docs/DEPLOYMENT.md`, `docs/DATABASE.md` | **partial** |
| 5 | Cold-start agent generating a plugin | A · C · L | `schemas/`, `skills/ductile/`, `plugins/echo/` | **partial** |
| 6 | In-repo coding agent | A · C · E | `AGENTS.md`, `internal/docs/lint_test.go` | **served** |
| 7 | Agent doing first-time setup or config generation | A · O · L | none canonical | **gap** |
| 8 | Agent operating a live Ductile instance | A · O · E | `/skills` registry, OpenAPI | **partial** |

**Status legend:** *served* — surface exists and works for this cell.
*partial* — surface exists but is incomplete, scattered, or known-stale.
*gap* — net-new content or feature is needed.
*deferred* — work is parked with explicit scope (see Coverage below).

---

## One-paragraph stories

**1. H · C · L — first-time contributor.**
Cloned the repo to add a plugin for service X. Wants a 30-minute path from
clone to working plugin on their machine, learning idioms by doing rather
than by reading 800-line specs. Needs: a "start here" page ≤ 5 minutes of
reading, one runnable teaching plugin, vocabulary table cross-linked so
terms acquire meaning during the walkthrough.

**2. H · C · E — maintainer.**
Changing the queue, router, or protocol. Wants design decisions, invariants,
and trade-offs in one click so they don't break a constraint they didn't know
existed. Needs: authoritative architecture doc (today), per-package design
notes for load-bearing packages (today: gap), enumerated non-negotiable
constraints (today: `AGENTS.md §3d`).

**3. H · O · L — evaluator.**
Just heard about Ductile. Wants a five-minute path from install to seeing a
real event flow, *before* reading documentation, so they can decide whether
Ductile fits. Needs: a working minimal example, generated config (not
copy-paste), one screenshot or asciinema of a live event flow (the
standalone `ductile-watch` TUI is under redesign; see
`ductile-hickey-tui-rip-and-rewrite` working note).

**4. H · O · E — veteran operator.**
Running Ductile on Unraid or a homelab. Wants runbooks for failure modes —
orphaned jobs, integrity check failure, plugin crash loop, full disk — so
they can recover without reading source. Needs: operator runbook organised
by *symptom*, not by feature.

**5. A · C · L — cold-start agent generating a plugin.**
Invoked with no prior Ductile context. Wants to discover the plugin contract
from machine-readable schemas and one canonical example, so it can produce a
valid plugin without hallucinating field names. Needs: schemas at a stable
path, a reference plugin explicitly tagged as the teaching example.

**6. A · C · E — in-repo coding agent.**
Working inside the repo on a real change. Wants the contract — style, safety,
idioms, vocabulary — in one file at a predictable path. Needs: `AGENTS.md`
(now done), per-package design notes (overlaps with persona 2), a `bd`
workflow it can drive (already present).

**7. A · O · L — agent doing first-time setup.**
Helping a user adopt Ductile. Wants a deterministic way to generate a valid
initial config and verify it, so it never produces config that fails
`ductile config check`. Needs: `ductile init`, advertised use of
`schemas/config.schema.json`, a curated `examples/` library.

**8. A · O · E — agent operating live Ductile.**
Building automations against a running instance. Wants every operation
discoverable through `/skills` and OpenAPI, so it does not need to read prose
docs to act safely. Needs: live OpenAPI, `/skills` as the *primary* surface
for this cell, an agent-readable runbook for recovery flows.

---

## Cross-cutting design implications

- **Audiences share information; they need different forms.** The same fact
  ("baggage propagates downstream") needs narrative form for Learner cells,
  reference form for Expert cells, and machine-readable form for Agent cells.
  A doc plan that ignores this rebuilds the same content three times by
  accident; a deliberate plan maintains it once and projects it three ways.

- **Coder ↔ Operator is the cleanest domain split** and maps naturally to
  the existing `internal/` vs `~/.config/` boundary.

- **Learner ↔ Expert is information density.** A document trying to be both
  serves neither. Tutorial content and reference content should not share
  files.

- **Agent ↔ Human is a form question.** Agent surfaces (`schemas/`,
  `skills/`, OpenAPI, `AGENTS.md`) are not separate documentation; they are
  the same content in machine-actionable form. They deserve first-class
  billing alongside `docs/`, not inside it.

- **Two cells unblock from the same work.** Personas 3 (H·O·L) and 7 (A·O·L)
  both fail today for the same reason: there is no canonical, validated
  minimal config. A `ductile init` plus a curated `examples/` library serves
  both.

- **One cell is a content gap, not a layout gap.** Persona 4 (H·O·E) needs
  symptom-organised runbooks that do not exist anywhere today. No
  reorganisation produces them.

---

## Coverage today

| Cell | Status | Notes |
|---|---|---|
| 1 H·C·L | partial | No signposted "first 30 minutes" path; idioms now live in `AGENTS.md` but the entry experience does not yet guide a learner there. |
| 2 H·C·E | served | `AGENTS.md` (vocabulary, design grounding, constraints) plus `docs/ARCHITECTURE.md` cover the steady-state need. Per-package design notes would strengthen it. |
| 3 H·O·L | partial / **deferred (Phase 3)** | Root `config.yaml` is the welcome-mat and is known-stale. Rethinking the exemplar is parked: `ductile init` + curated `examples/`. Watch TUI ripped pending `ductile-watch` rewrite (see `ductile-hickey-tui-rip-and-rewrite`). |
| 4 H·O·E | partial / **gap** | Operator guide and deployment exist; symptom-driven runbooks do not. Net-new content. Interim observability is the API and structured logs; `ductile-watch` redesign tracked in `ductile-hickey-tui-rip-and-rewrite`. |
| 5 A·C·L | partial | Schemas exist but are not advertised; no plugin is explicitly labelled as the canonical teaching example. |
| 6 A·C·E | served | Unified `AGENTS.md` is the contract; doc-smoke lint covers it. |
| 7 A·O·L | **gap / deferred (Phase 3)** | Same unblock as persona 3. |
| 8 A·O·E | partial | `/skills` and OpenAPI exist; agent-readable recovery runbooks do not. |

---

## How to use this document

- **As a reader:** find your cell. Follow its landing surface. If your cell
  is marked *partial* or *gap*, the documentation cannot fully serve you yet
  and the gap is acknowledged here.

- **As a contributor proposing a change to docs or affordances:** name the
  cells it serves and the cells it does not. A change that serves a *gap*
  cell is high-leverage; a change that re-paints a *served* cell needs
  stronger justification.

- **As a reviewer:** cite cells in review comments. *"This is for cell 4,
  currently a gap"* is a precise statement; *"this is too detailed for
  beginners"* is not.

- **As a maintainer:** when the doc taxonomy changes, re-validate this file.
  Every cell must still resolve to a landing surface (or be honestly marked
  *gap* / *deferred*).

---

## Maintenance

This file is referenced from `AGENTS.md` and `CONTRIBUTING.md` and is
expected to evolve alongside the doc taxonomy. Coverage status should be
updated whenever a cell's landing surface materially changes. If a cell goes
unserved for a release without being marked *deferred*, that is a planning
signal, not a documentation signal.
