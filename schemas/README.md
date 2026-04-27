---
audience: [5, 7, 8]
form: agent-surface
density: expert
verified: 2026-04-27
---

# Ductile JSON Schemas

Machine-readable contracts for every Ductile configuration and runtime
artefact. Agents and tooling resolve these by repo-relative path; there is
no stable HTTP hosting commitment (resolved in
[`../docs/DOCS_RETHINK.md`](../docs/DOCS_RETHINK.md) §7.2).

| Schema | Validates | Canonical doc |
|---|---|---|
| [`config.schema.json`](config.schema.json) | `config.yaml` — the root configuration file | [`../docs/CONFIG_REFERENCE.md`](../docs/CONFIG_REFERENCE.md) |
| [`include.schema.json`](include.schema.json) | Any YAML fragment loaded via the `include:` mechanism | [`../docs/CONFIG_REFERENCE.md`](../docs/CONFIG_REFERENCE.md) |
| [`plugins.schema.json`](plugins.schema.json) | `plugins.yaml` — per-plugin runtime configuration | [`../docs/CONFIG_REFERENCE.md`](../docs/CONFIG_REFERENCE.md) |
| [`pipelines.schema.json`](pipelines.schema.json) | `pipelines.yaml` — pipeline DSL definitions | [`../docs/PIPELINES.md`](../docs/PIPELINES.md) |
| [`routes.schema.json`](routes.schema.json) | `routes.yaml` — event routing rules | [`../docs/ROUTING_SPEC.md`](../docs/ROUTING_SPEC.md) |
| [`plugin-manifest.schema.json`](plugin-manifest.schema.json) | `plugins/<name>/manifest.yaml` — per-plugin manifest | [`../docs/PLUGIN_DEVELOPMENT.md`](../docs/PLUGIN_DEVELOPMENT.md) |
| [`tokens.schema.json`](tokens.schema.json) | `tokens.yaml` — API bearer tokens and scopes | [`../docs/CONFIG_REFERENCE.md`](../docs/CONFIG_REFERENCE.md) |
| [`webhooks.schema.json`](webhooks.schema.json) | `webhooks.yaml` — standalone webhook endpoints | [`../docs/WEBHOOKS.md`](../docs/WEBHOOKS.md) |

Cross-field rules (for example `plugins.<name>.parallelism ≤
service.max_workers`) are enforced by runtime validation, not purely by
JSON Schema. Treat the schemas as the shape contract and the canonical
docs as the semantics.
