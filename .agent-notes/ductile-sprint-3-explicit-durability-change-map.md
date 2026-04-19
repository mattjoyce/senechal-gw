# Ductile Sprint 3 Explicit Durability Change Map

Date: 2026-04-19
Branch: `hickey-sprint-3-explicit-durability`
Source note: `/home/matt/Obsidian/Personal1/ductile-hickey-sprint-3-explicit-durability.md`
Issue: `ductile-6gp`

## Current Baseline

- Branch was created from updated `main` at `8463e86`.
- Existing pipeline DSL supports `with:` on `uses` steps, but has no `baggage:` field.
- Existing plugin manifests describe commands, schemas, idempotency, and retry-safety hints, but not emitted event metadata or baggage namespaces.
- `internal/state.ContextStore.Create` owns event-context accumulation and currently shallow-merges child updates into parent context.
- `internal/dispatch.routeEvents` currently derives context updates from the full routed event payload via `payloadObjectFromEvent`.
- API-triggered pipeline roots currently seed root baggage from the trigger event payload.
- Config snapshots still describe the old `origin_keys_only` baggage immutability semantics.
- Docker fixture harness exists under `scripts/test-docker*` and `test/fixtures/docker/*`; persistent non-prod environment does not exist yet.

## Design Target

Sprint 3 should make this mechanically true:

> Plugins emit facts. Authors name durable facts. Core enforces immutability.

Data surfaces must be separate:

- `payload`: per-hop event data for the immediate downstream job.
- `with:`: request-payload shaping before plugin spawn.
- `baggage:`: explicit promotion of values into durable context.
- `context`: accumulated immutable named values.

Legacy automatic payload-to-baggage promotion remains only as a transition mode and must become observable.

## Code Surfaces To Change

### 1. Pipeline DSL

Files:

- `internal/router/dsl/types.go`
- `internal/router/dsl/compiler.go`
- `internal/router/dsl/compiler_test.go`
- `internal/router/dsl/loader.go`

Needed changes:

- Add a `baggage:` field to `StepSpec` and compiled `Node`.
- Support two forms:
  - explicit path mappings, e.g. `summary.text: payload.text`
  - bulk import, e.g. `from: payload` plus optional `namespace: transcript`
- Reject `baggage:` on non-`uses` steps for Sprint 3 unless a concrete `call` inheritance rule is designed.
- Include baggage declarations in pipeline fingerprinting.
- Validate obvious invalid forms during compile/load:
  - bulk import without explicit namespace and without resolvable upstream/default namespace
  - invalid baggage path syntax
  - conflicting shorthand forms if the chosen YAML shape needs disambiguation

Open implementation detail:

- The YAML parser may need a custom type for `baggage:` so explicit mappings and bulk import can coexist without ambiguous `map[string]string` handling.

### 2. Plugin Manifest Namespace Defaults

Files:

- `internal/plugin/manifest.go`
- `internal/plugin/discovery.go`
- `internal/plugin/*_test.go`
- plugin fixture manifests

Needed changes:

- Add optional emitted-event metadata under command declarations.
- Minimum shape:

```yaml
commands:
  - name: handle
    type: write
    emits:
      - type: whisper.transcribed
        baggage_namespace: whisper
```

- Expose namespace metadata on discovered `plugin.Plugin`.
- Preserve compatibility for manifests without `emits`.
- Validate namespace syntax if present.

Open implementation detail:

- Alias plugins (`config.plugins.<name>.uses`) should probably inherit base manifest namespace defaults unless explicitly overridden by future config. Sprint 3 can keep alias behavior read-only.

### 3. Baggage Claim Evaluation

Likely new package or file:

- `internal/dispatch/baggage.go`
- `internal/dispatch/baggage_test.go`

Needed behavior:

- Evaluate claim expressions against a scope containing:
  - `payload.*`
  - `context.*`
  - probably `event.*` for metadata if useful
- Reuse existing `with:` expression machinery where practical, but keep the concepts separate.
- Explicit path mapping writes exactly the named baggage path.
- Bulk import copies the selected source object under the namespace object.
- Missing explicit mapping source should fail loudly unless a concrete optional-value convention is designed.
- Bulk import source must evaluate to a JSON object.

Open implementation detail:

- Current `with:` templates return strings. Baggage must preserve JSON values. If existing template evaluation cannot preserve types, baggage needs a small path evaluator rather than string templating.

### 4. Context Store Deep Accretion

Files:

- `internal/state/context_store.go`
- `internal/state/context_store_test.go`

Needed changes:

- Replace shallow merge and `origin_*` immutability with deep accretion.
- Add typed immutable-path error, likely `ErrBaggagePathImmutable`.
- Error text should include the rejected path and avoid dumping values.
- Allow adding new children under existing objects.
- Allow exact structural repeats.
- Reject scalar/object conflicts.
- Keep max context size enforcement.
- Keep lineage behavior unchanged.

Compatibility concern:

- Historical `event_context` rows are read as-is. Do not rewrite existing rows.

### 5. Routing And Context Creation

Files:

- `internal/dispatch/dispatcher.go`
- `internal/router/interface.go`
- `internal/router/engine.go`
- `internal/router/engine_test.go`
- `internal/e2e/*`

Needed changes:

- Stop treating `payloadObjectFromEvent(next.Event)` as the only context update source.
- Create context updates from:
  - explicit `baggage:` claims
  - transition-mode legacy auto-promotion, separately marked or diagnosed
- Keep routed event payload unchanged for immediate downstream job.
- Keep `with:` application as request-payload shaping before plugin spawn.
- Keep root pipeline trigger baggage behavior initially, but consider whether root context should also move to explicit origin/durable seed rules later.
- Avoid making core transport fields durable by accident:
  - `result`
  - `ductile_upstream_plugin`
  - `ductile_upstream_pipeline`
  - `ductile_upstream_step_id`

Transition-mode requirement:

- Legacy payload auto-promotion should still work temporarily, but diagnostics must identify it so configs can be migrated.

### 6. Config Snapshot Semantics

Files:

- `internal/configsnapshot/snapshot.go`
- `internal/configsnapshot/snapshot_test.go`
- `internal/inspect/report.go`
- `internal/inspect/report_test.go`

Needed changes:

- Replace old baggage semantics with:

```json
{
  "baggage_immutability": "deep_accretion",
  "baggage_durability": "legacy_payload_auto_promote_with_explicit_claims"
}
```

- Later strict mode should become:

```json
{
  "baggage_immutability": "deep_accretion",
  "baggage_durability": "explicit_claims_only"
}
```

- `job inspect` should make active semantics visible enough for migration diagnosis.

### 7. Prediction / Migration Diagnostics

Possible files:

- `internal/inspect/report.go`
- `cmd/ductile/main.go`
- new command or report helper

Needed behavior:

- Identify current auto-promoted payload keys along routes/pipeline edges.
- Flag generic/transport keys:
  - `result`
  - `status`
  - `message`
  - `path`
  - `file_path`
  - `url`
  - `content`
  - `ductile_upstream_plugin`
  - `ductile_upstream_pipeline`
  - `ductile_upstream_step_id`
- Classify as:
  - durable fact needing explicit baggage claim
  - local payload field suitable for `with:`
  - transport/debug field that should not be durable
  - ambiguous field needing author review

This can start as an inspect/report aid rather than a perfect static analyzer.

### 8. Docs

Files:

- `docs/PIPELINES.md`
- `docs/PLUGIN_DEVELOPMENT.md`
- `docs/PLUGIN_DIAGNOSTICS.md`
- `docs/CONFIG_REFERENCE.md`
- `docs/ARCHITECTURE.md`
- `docs/GLOSSARY.md`

Needed changes:

- Replace "every event payload becomes context" language.
- Document `payload` vs `with:` vs `baggage` vs `context`.
- Document namespace defaults and bulk import.
- Document deep-accretion immutability.
- Document transition-mode diagnostics and migration expectations.

## Deterministic Docker Fixture

Add a fixture under:

```text
test/fixtures/docker/hickey-sprint-3/
```

It should include:

- config directory with API enabled
- small shell/Python plugins with manifests
- pipeline file exercising explicit baggage
- `run.sh` compatible with `scripts/test-docker-runner`

Minimum scenarios:

- explicit mapping creates nested baggage
- bulk import uses upstream default namespace
- explicit namespace overrides default
- missing namespace/default fails config validation
- `with:` shapes payload without creating baggage
- same path/same value repeat succeeds
- same path/different value fails with typed diagnostic
- adding a new child under existing namespace succeeds
- legacy auto-promotion is visible in compatibility mode
- `result` and `ductile_upstream_*` are not silently treated as durable facts

## Persistent Non-Prod Docker

Useful path:

```text
nonprod/hickey-sprint-3/
```

This should be persistent and production-shaped, not a disposable fixture.

Useful inputs from Matt:

- redacted production-like `config/`
- redacted/safe plugin tree or representative subset
- representative seed/replay events
- list of external side effects that must be stubbed or redirected
- safe tokens or test auth config
- expected outputs for known-good pipeline runs

Repo-side contents should include:

- `docker-compose.yml`
- `config/`
- `plugins/`
- `state/` volume mount
- `workspaces/` volume mount
- `seed/` or `replay/` scripts
- README/runbook for start, stop, reset, inspect, and backup

Use it for:

- baseline run on updated `main`
- Sprint 3 branch run with same seeds
- comparison of context lineage, auto-promotion diagnostics, failed jobs, and operator-facing messages

## Suggested Work Slices

1. DSL/model support for `baggage:` plus tests.
2. Manifest namespace defaults plus tests.
3. Deep-accretion `ContextStore` plus tests.
4. Baggage claim evaluator plus tests.
5. Route/context creation integration in compatibility mode.
6. Deterministic Docker fixture.
7. Prediction/inspect diagnostics.
8. Non-prod Docker baseline and comparison workflow.
9. Docs and migration runbook.

Retry policy is intentionally excluded from this branch's priority path; see `/home/matt/Obsidian/Personal1/ductile-hickey-retry-policy-split.md`.
