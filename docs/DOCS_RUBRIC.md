---
audience: [2, 6]
form: meta
density: expert
verified: 2026-04-27
---

# Documentation Rubric

The standard every canon document in this repository is held to. This file
is referenced by `AGENTS.md` and is enforceable: contributors apply it,
reviewers cite it, lint can check most of it.

It exists because `AUDIENCES.md` defines *who* docs serve and
`DOCS_RETHINK.md` defines *what* the docs should become. Without a rubric,
both rot. The rubric is how persona commitments survive contact with
ongoing change.

This document is itself bound by the rubric (see front-matter above).

---

## 1. The governing principle

> **Reader mode governs layout. Form follows reader mode. Density does
> not mix.**

Three reader modes appear in this project:

| Mode | What the reader is doing | Layout reward |
|---|---|---|
| **Learning** | Building a mental model from scratch | Coherent narrative; one document per thread. |
| **Looking up** | Already understands; needs a fact | Coherent reference, scannable; ctrl-F over traversal. |
| **Dispatching** | Acting under stress, entering at a symptom | Isolated rituals; one symptom one file; stable IDs. |

Layout is not a stylistic preference. It is determined by which mode the
reader is in.

The worked example: **runbooks vs reference**. A reference reader is
looking up; siblings adjacent are *helpful* (you scan, you map). A runbook
reader is dispatching; siblings adjacent are *hazardous* (you might
confuse this symptom with that one). Same project, opposite layout, same
principle.

---

## 2. Forms

Every canon document declares its `form` in front-matter. The form
constrains its density and its layout.

| Form | Reader mode | Density | Layout | Examples |
|---|---|---|---|---|
| `tutorial` | learning | learner | coherent narrative; one file per thread | `docs/learn/first-plugin.md` |
| `reference` | looking up | expert | coherent long-form; scannable; anchored sections | `docs/PIPELINES.md`, `docs/CONFIG_REFERENCE.md` |
| `runbook` | dispatching | expert | isolated; one symptom per file; stable ID | `docs/runbooks/orphaned-jobs.md` |
| `design` | looking up (rationale) | expert | coherent narrative; one file per concern | `internal/<pkg>/DESIGN.md`, `docs/ARCHITECTURE.md` |
| `agent-surface` | machine-actionable | n/a | schema- or skill-shaped | `schemas/*.json`, `skills/ductile/SKILL.md` |
| `meta` | governance | expert | coherent | `AUDIENCES.md`, this file, `DOCS_RETHINK.md` |

A document is exactly one form. A page that wants to be both `tutorial`
and `reference` becomes two pages.

---

## 3. Required front-matter

Every canon document carries this block. It is the contract.

```yaml
---
audience: [1, 2]            # cells from AUDIENCES.md (one or more)
form: reference             # one of: tutorial | reference | runbook | design | agent-surface | meta
density: expert             # learner | expert
verified: 2026-04-27        # ISO date of last reality-check
---
```

Optional, used where they earn their keep:

```yaml
coupled_to:                 # code paths whose change requires re-verifying this doc
  - internal/dispatch/
  - internal/router/

supersedes: RB-003          # for runbooks retired in favour of this one
runbook_id: RB-007          # for runbooks only; stable identity
```

**What the rubric does not include**, deliberately: tags, categories,
ownership-by-person, navigation hints, ordering keys, status flags. The
rubric stays Markdown-portable and machine-checkable.

---

## 4. Form × density legality

Not every combination is legal. Density tracks reader mode; mismatch is
the failure that produced today's drift.

| Form | Legal density | Rationale |
|---|---|---|
| `tutorial` | learner | Tutorials are for readers building a model. Expert tutorials are reference docs in disguise. |
| `reference` | expert | Reference is for readers who already understand. Learner reference is a tutorial in disguise. |
| `runbook` | expert | Dispatch mode assumes vocabulary; teach the vocabulary in tutorials. |
| `design` | expert | Rationale is for readers who already know the system. |
| `agent-surface` | n/a | Density does not apply to schemas. |
| `meta` | expert | Governance documents are for maintainers. |

A learner-flavoured paragraph in a reference document is a rubric
violation. So is a reference table dumped into a tutorial. The fix is
always to split, never to compromise density.

---

## 5. Linking discipline

- **Link by concept anchor, never by file path alone.** Use
  `pipelines.md#with-mapping`, not `pipelines.md` if a specific concept is
  meant. Anchors are stable across edits; a file's body is not.
- **Vocabulary terms link to `GLOSSARY.md` on first use** in any
  human-readable document. First use only; subsequent uses do not.
- **Up-links and down-links are explicit.** A runbook links *up* to
  `OPERATOR_GUIDE.md` for context. A tutorial links *down* to references
  it graduates the learner toward. Sibling sprawl is discouraged.
- **No wiki-style `[[brackets]]`.** Standard Markdown only. Obsidian
  habits stay in Obsidian.
- **External URLs declare why they are stable.** A linked GitHub issue,
  RFC, or vendor doc must have a reason to outlive the link (versioned
  release, archived page, etc.). Otherwise paraphrase in-line.

---

## 6. Coupling and freshness

Some documents lie about the system if the system changes underneath
them. They declare `coupled_to:` paths in front-matter.

The rule: **if a PR touches a coupled path, the PR re-verifies the
coupled doc and updates `verified:`.** Re-verifying is a real act
(read the doc against the changed code), not a date bump.

Initial coupling table (lives in front-matter on the docs themselves;
restated here for visibility):

| Document | Coupled to |
|---|---|
| `API_REFERENCE.md` | `internal/api/` |
| `CONFIG_REFERENCE.md` | `internal/config/` |
| `PIPELINES.md` | `internal/dispatch/`, `internal/router/` |
| `ROUTING_SPEC.md` | `internal/router/` |
| `PLUGIN_DEVELOPMENT.md` | `internal/protocol/`, `internal/plugin/` |
| `PLUGIN_FACTS.md` | `internal/state/` |
| `SCHEDULER.md` | `internal/scheduler/`, `internal/scheduleexpr/` |
| `WEBHOOKS.md` | `internal/webhook/` |
| `DATABASE.md` | `internal/storage/`, `internal/queue/` |
| `VERSIONING.md` | `scripts/version.sh`, `Dockerfile` |
| Schemas (`schemas/*.json`) | their respective subsystems |

**Freshness window.** A coupled document with `verified:` older than
**90 days** is a rubric violation. Lint warns at 60 days, fails at 90.
Non-coupled documents have no automatic freshness pressure; they are
verified opportunistically.

---

## 7. Runbook conventions

Runbooks are the rubric's hardest case because they exist for the
hardest moment (the 3am dispatch). They earn their own short ruleset.

- **One symptom per file.** Adjacent content is hazard, not help.
- **Stable ID** (`runbook_id: RB-NNN`) referenced from monitoring
  alerts, beads cards, post-mortems, and the skill.
- **Self-contained.** Every command, check, and rollback in the file.
  If the responder must open a second tab mid-recovery, the runbook
  failed.
- **Standard shape:**
  1. **Symptom** — what the operator sees.
  2. **How to confirm** — fast checks that this is actually the right
     runbook, not a similar-looking failure.
  3. **Likely causes** — short, ranked.
  4. **Recovery steps** — numbered, idempotent where possible, with
     the exact commands.
  5. **Rollback** — what to do if recovery makes it worse.
  6. **Prevention** — the post-incident note for the next reader.
- **Independently testable.** A runbook should be exercisable against a
  fixture: inject the failure, run the runbook, assert recovery. New
  runbooks land with a fixture where practical.
- **Retirement.** When a code change makes the failure mode impossible,
  the runbook gets a `superseded:` flag and an archive date. It does not
  silently disappear.

---

## 8. What canon means

Canon is anything in `docs/`, `schemas/`, `skills/`, and the project's
top-level governance files (`AGENTS.md`, `CONTRIBUTING.md`, `README.md`).
Canon is bound by the rubric.

Ephemera — RCAs, sprint logs, bench write-ups, working notes,
proposals — is not canon and not bound by the rubric. Ephemera lives in
`notes/` (versioned, dated, explicitly non-canon), in beads attachments,
or in personal Obsidian. Ephemera that has earned canon promotion is
rewritten to the rubric, not migrated as-is.

---

## 9. Lint rules

The rubric is enforceable. Lint runs in `scripts/test-fast`.

| Rule | Strictness | Phase |
|---|---|---|
| Required front-matter present | hard fail | Phase 1 |
| `audience` cells exist in `AUDIENCES.md` | hard fail | Phase 1 |
| `form` is one of the legal values | hard fail | Phase 1 |
| `form` × `density` combination is legal (see §4) | hard fail | Phase 1 |
| `coupled_to:` paths exist on disk | hard fail | Phase 1 |
| `runbook_id:` is unique across canon | hard fail | Phase 3 |
| Internal links resolve (file + anchor) | hard fail | Phase 1 |
| `verified:` within 60 days for coupled docs | warn | Phase 1 |
| `verified:` within 90 days for coupled docs | hard fail | Phase 2 |
| Glossary terms linked on first use | warn | best-effort |

The lint is implemented as a Go test (`internal/docs/rubric_test.go`)
following the precedent of the existing doc-smoke lint, so failures are
visible in normal `go test ./...` runs.

---

## 10. Adoption

The rubric does not retroactively fail every existing doc. Adoption is
phased:

1. **Phase 1 of `DOCS_RETHINK.md`** — front-matter added to every canon
   doc; lint enforces presence and validity. No `verified:` enforcement
   yet beyond presence.
2. **Phase 2** — density splits land; `form` × `density` legality
   enforced.
3. **Phase 3** — runbooks land with `runbook_id:` and the runbook
   shape; `runbook_id:` uniqueness enforced.
4. **Phase 5** — package `DESIGN.md` notes adopt the rubric from day
   one; the first one sets the template.

Until a phase lands, the rubric documents the *target*, not the
*current state*. Documents are conformant or non-conformant on a
schedule, not all at once.

---

## 11. Maintenance

This rubric evolves when persona needs evolve, not when an individual
doc is awkward to write. Pressure to weaken a rule because a doc is
hard to fit into it is feedback that the doc has the wrong form, not
that the rubric is wrong.

When the rubric changes, every cell in `AUDIENCES.md` is re-checked
against the new rule. Changes here propagate to `AGENTS.md §6` and to
the lint. The cost of changing the rubric is real; that is the point.
