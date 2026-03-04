# ZK Update — Concurrency Rollout + Schemas (2026-03-04)

## Shipped to `main`
- Per-plugin + global concurrency controls
  - `service.max_workers`
  - `plugins.<name>.parallelism`
- Dispatcher refactor to bounded worker coordinator (parallel execution)
- Queue eligible dequeue filtering (`DequeueEligible`) with plugin saturation skip
- Per-target guard via dedupe key (same key not concurrent)
- `github_repo_sync` emits per-repo dedupe key for git sync fan-out
- Manifest hint: `concurrency_safe: true|false`
  - false => serial by default unless explicitly overridden in config
- Marked functional-state plugins as `concurrency_safe: false`
  - `file_watch`, `folder_watch`, `youtube_playlist`, `jina-reader`

## Bench evidence
- CPU and IO matrices (serial vs capped vs parallel)
- Mixed max-core run (100 jobs) successful
- Mini soak (5 min) successful: 450/450, zero retries/errors
- Benchmark report: `docs/CONCURRENCY_BENCH_2026-03-04.md`
- Raw artifacts: `stress-test/artifacts/`

## Schemas merged to `main`
- `schemas/config.schema.json`
- `schemas/include.schema.json`
- `schemas/plugins.schema.json`
- `schemas/pipelines.schema.json`
- `schemas/tokens.schema.json`
- `schemas/webhooks.schema.json`
- `schemas/routes.schema.json`
- `schemas/plugin-manifest.schema.json`

## Remaining follow-ups
- `ductile-pjo` (open): productize automated instrumentation/metrics extraction
- `ductile-sf0` (open): remove non-functional plugin state writes
