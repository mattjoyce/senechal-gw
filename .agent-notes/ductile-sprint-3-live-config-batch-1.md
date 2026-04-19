# Sprint 3 Live Config Migration Batch 1

Date: 2026-04-19
Branch: `hickey-sprint-3-explicit-durability`
Live config source: `/home/matt/.config/ductile/pipelines.yaml`
Proposed config copy: `.agent-notes/ductile-sprint-3-live-config-batch-1.proposed.yaml`

This is a reviewed migration artifact only. It does not edit the live config.

Status: do not deploy yet. The interim value-name contract should be filled in
for the relevant external plugins first, so the author has manifest-declared
consumed and emitted payload names rather than code-derived guesses.
`input_schema` remains legacy during the transition.

## Batch Scope

Batch 1 migrates two pipelines:

- `astro-rebuild-staging-on-summary-change`
- `web-summarize`

These cover the two shapes already exercised in non-prod:

- notification chain: producer names durable cause, notifier shapes request
  from `context.*`
- content chain: fetch -> summarize -> write, without making raw fetched
  content durable

## Why These First

They are small, high-signal, and expose the main authoring questions:

- generic fields such as `result`, `content`, `url`, and `path` need domain
  names at the baggage boundary
- `with:` remains request shaping and can read `context.*`
- durable facts should follow actual plugin output contracts

## Proposed Changes

### `astro-rebuild-staging-on-summary-change`

Root step claims folder-watch facts:

```yaml
baggage:
  astro.summaries.watch_id: payload.watch_id
  astro.summaries.root: payload.root
  astro.summaries.snapshot_hash: payload.snapshot_hash
  astro.summaries.changed_count: payload.changed_count
  astro.summaries.created: payload.created
  astro.summaries.modified: payload.modified
  astro.summaries.deleted: payload.deleted
```

Notify step shapes the Discord request from durable context:

```yaml
with:
  message: "Staging rebuilt — {context.astro.summaries.changed_count} summary file(s) updated."
```

Notify step claims only the rebuild wrapper facts that
`astro_rebuild_staging` actually emits:

```yaml
baggage:
  rebuild.command: payload.command
  rebuild.exit_code: payload.exit_code
  rebuild.duration_ms: payload.duration_ms
  rebuild.stdout_truncated: payload.stdout_truncated
  rebuild.stderr_truncated: payload.stderr_truncated
  rebuild.working_dir: payload.working_dir
  rebuild.executed_at: payload.executed_at
```

It deliberately does not claim `payload.result`; the rebuild wrapper does not
emit `result` in the routed event payload.

### `web-summarize`

Fetch step claims only the root URL:

```yaml
baggage:
  web.input_url: payload.url
```

Summarize step claims fetch metadata, not the fetched body:

```yaml
baggage:
  web.url: payload.url
  web.content_hash: payload.content_hash
  web.truncated: payload.truncated
```

Write step claims Fabric output facts:

```yaml
baggage:
  summary.text: payload.result
  summary.pattern: payload.pattern
  summary.model: payload.model
  summary.input_length: payload.input_length
  summary.output_length: payload.output_length
```

It deliberately does not claim:

- `web.content`
- `file.output_path`
- `file.filename`

Those are either large/transient or optional in the live event stream. Missing
source paths are errors under Sprint 3 explicit baggage, so optional fields need
a later optional-claim primitive or stronger event contracts.

## Validation

The proposed file was validated with the branch DSL compiler using the current
live plugin roots/config.

Command:

```bash
rm -rf /tmp/ductile-sprint3-batch1-config
cp -a /home/matt/.config/ductile /tmp/ductile-sprint3-batch1-config
cp .agent-notes/ductile-sprint-3-live-config-batch-1.proposed.yaml /tmp/ductile-sprint3-batch1-config/pipelines.yaml
go run ./cmd/ductile config lock --config-dir /tmp/ductile-sprint3-batch1-config
go run ./cmd/ductile config check --config-dir /tmp/ductile-sprint3-batch1-config
go test ./internal/router/dsl
```

Result:

```text
Configuration valid (15 warning(s))
ok   github.com/mattjoyce/ductile/internal/router/dsl
```

The config warnings are existing duplicate/unused plugin discovery warnings in
the copied live config environment; they are not baggage syntax failures.

## Non-Prod Evidence Behind This Batch

Report:

```text
/home/matt/admin/nonprod/ductile-hickey-sprint-3/artifacts/sprint-3-live-shape-migration-report.md
```

Focused replay result:

```text
2 seeds accepted
5 jobs succeeded
0 jobs failed
5 event_context rows
```

Baseline guard:

```text
8 seeds accepted
16 jobs succeeded
0 jobs failed
16 event_context rows
```

Important finding carried forward:

- authors must map actual plugin output contracts
- for `sys_exec`-style wrappers, JSON printed to stdout remains in
  `payload.stdout`; it is not parsed into top-level payload fields by core

## Apply Plan

1. Review `.agent-notes/ductile-sprint-3-live-config-batch-1.proposed.yaml`.
2. If accepted, copy only the two changed pipeline sections into
   `/home/matt/.config/ductile/pipelines.yaml`.
3. Reload/restart Ductile on the Sprint 3 branch.
4. Trigger one `astro.summaries.changed` and one `web.url.detected` event.
5. Inspect `event_context.accumulated_json` for:
   - no raw root payload leakage on opted-in steps
   - no durable `web.content`
   - named `astro.summaries.*`, `rebuild.*`, `web.*`, and `summary.*` facts
