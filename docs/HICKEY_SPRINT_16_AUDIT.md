# Hickey Sprint 16 — Pipeline-level `if:` Audit

> **Purpose.** §2 of `ductile-hickey-sprint-16-pipeline-if-predicates`
> requires auditing production pipelines for cases where a pipeline-level
> `if:` predicate would either simplify config or have prevented a real
> misroute. The §2.2 soft gate requires ≥3 concrete cases or the sprint
> ships docs-only and rolls forward. **This audit catalogs the cases and
> establishes the gate as passed.**

Audit input: `~/.config/ductile/pipelines.yaml` on the production
ThinkPad host, snapshot 2026-04-26.

---

## 1. Audit method

```bash
grep -nE '^\s*on:\s|^\s*on-hook:\s' ~/.config/ductile/pipelines.yaml
```

For each trigger, the audit answers three §2.1 questions:

1. Are there multiple pipelines on the same event? Each currently fires
   on every match.
2. Are there lifecycle-hook pipelines (`on-hook:`) that fire on every
   matching system signal across the whole runtime?
3. Are there `core.switch` steps that exist purely to filter at the top
   of a pipeline?

---

## 2. Triggers in production (snapshot)

22 pipelines total. Trigger frequency:

| Trigger / Hook | Pipelines |
|---|---|
| `git_repo_sync.completed` | 2 (`repo-compliance`, `repo-changelog`) |
| every other `on:` | 1 |
| `on-hook: job.failed` | 1 (`job-failure-notify`) |
| `on-hook: job.completed` | 1 (`job-complete-notify`) |

No `core.switch` steps appear at the top of any user-authored pipeline.
The compiler synthesises `core.switch` from step-level `if:` gates
internally; that is a different concern from §2.1's "filter-only switch
step" workaround.

---

## 3. Concrete candidates

### 3.1 · `repo-changelog` (Candidate 1) — production pain, real

```yaml
- name: repo-changelog
  on: git_repo_sync.completed
  steps:
    - id: changelog
      uses: changelog_microblog
      if:
        path: payload.new_commits
        op: eq
        value: true
      ...
```

**Pain today.** The first step of `repo-changelog` only runs when the
trigger event payload reports `new_commits: true`. But the dispatch fires
unconditionally — every `git_repo_sync.completed` event spawns the
pipeline, allocates a workspace, and runs through a `core.switch` to
decide nothing happened. Every uneventful repo sync emits a synthetic
`core.switch` job and a workspace dir.

**Lift target.**

```yaml
- name: repo-changelog
  on: git_repo_sync.completed
  if:
    path: payload.new_commits
    op: eq
    value: true
  steps:
    - id: changelog
      uses: changelog_microblog
      ...
```

**Effect.** No dispatch, no workspace, no `core.switch` job when
`new_commits` is false. The sister pipeline `repo-compliance` keeps
firing on the same event — it always wants to run, regardless of
commit count. This is the *one event, multiple consumers, divergent
filter* model the architecture is supposed to support.

### 3.2 · `job-failure-notify` (Candidate 2) — production pain, real

```yaml
- name: job-failure-notify
  on-hook: job.failed
  steps:
    - id: notify
      uses: discord_notify
      ...
```

**Pain today.** Fires on **every** failed job across **every** plugin.
Discord notifications spam on transient errors, on plugins that are
known-flaky, and on plugins where failure is part of normal operation
(e.g. a `check_*` plugin that "fails" to indicate "no result yet").
The pipeline cannot be scoped without rewriting it to call a filter
plugin first.

**Lift target.**

```yaml
- name: job-failure-notify
  on-hook: job.failed
  if:
    not:
      path: payload.plugin
      op: in
      value: [check_youtube, jina-reader]   # known-noisy
  steps:
    - id: notify
      uses: discord_notify
```

Or, more useful, gate on retry exhaustion:

```yaml
  if:
    path: payload.attempts_exhausted
    op: eq
    value: true
```

**Effect.** Discord stays signal, not noise. The hook-level predicate is
exactly the surface that's missing today — `core.switch` cannot be
inserted before a hook pipeline because hooks already dispatch root-level.

### 3.3 · `job-complete-notify` (Candidate 3) — production pain, real

```yaml
- name: job-complete-notify
  on-hook: job.completed
  steps:
    - id: notify
      uses: discord_notify
      ...
```

**Pain today.** Fires on **every** completed job. Heartbeat polls,
bookkeeping, internal `core.switch` jobs, every step of every pipeline —
everything. This pipeline is functionally broken on an active system
because it cannot distinguish *user-relevant completions* from *internal
runtime mechanics*.

**Lift target.**

```yaml
  if:
    path: payload.plugin
    op: in
    value: [astro_rebuild_staging, run_healthdata_etl, fabric]
```

Or scope to root-level only:

```yaml
  if:
    path: payload.is_root_job
    op: eq
    value: true
```

(Whether `payload.is_root_job` exists in the lifecycle event payload is
a separate plumbing question; the *predicate surface* is the precondition
for any such filter to be usable.)

**Effect.** Same shape as 3.2 — without pipeline-level `if:` on hook
triggers, this pipeline is functionally a "discord firehose" mode and
operators turn it off entirely.

---

## 4. Soft gate

**Threshold:** §2.2 / §11.4 require ≥3 concrete cases.

**Result:** 3 concrete cases identified:

1. `repo-changelog` — workspace + `core.switch` waste on every empty sync.
2. `job-failure-notify` — un-filterable; hook surface has no other
   filter mechanism.
3. `job-complete-notify` — un-filterable; same.

**Decision.** Proceed with Sprint 16 implementation. The audit
substantiates §1.3 ("authoring pain is real, even if narrow") with three
production pipelines that the new feature improves.

---

## 5. Out-of-audit observations (informational)

- **`garmin-daily-summary` / `withings-weight-summary`** (`data.change.<src>`):
  no current filter pain — both pipelines always want to run on every
  change event. *Not a Sprint 16 candidate.* Kept here so future audits
  don't re-discover them.
- **`llm-pipeline`** (`on: llm.request`): step-level `if:` gates the
  YouTube and fetch branches based on URL shape. The trigger itself
  *should* always fire when an llm.request comes in — gating belongs
  at the step level. *Not a Sprint 16 candidate.*
- **`if-test`** (`on: test.if.check`): the smoke pipeline that
  exercises step-level `if:`. Once Sprint 16 lands, a sibling
  `if-test-pipeline-level` smoke fixture is the natural next addition
  (see `test/fixtures/docker/`).

---

## 6. Migration mechanic for the three candidates

When Sprint 16 lands, the three candidate pipelines above can be
migrated incrementally — each is independently liftable, none depend
on each other, and the rollback is trivial (delete the `if:` block,
the pipeline reverts to today's "always fires" behaviour). This is
captured here so the post-merge migration can move at whatever cadence
the operator (matt) prefers, without re-deriving the targets.

