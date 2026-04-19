# Sprint 3 Interim Plugin Value Names

Date: 2026-04-19
Branch: `hickey-sprint-3-explicit-durability`

## Decision

Add an interim names-only plugin manifest contract.

Plugins declare the author-facing payload contract through names-only
`values.consume` and `values.emit`. `input_schema` remains as the legacy typed
schema surface during the transition and can be removed later.

## Why

The explicit durability design puts naming responsibility on the pipeline
author, but authors need a visible local plugin contract to avoid reverse
engineering every mapping from code.

Names-only values give us a useful sanity test:

- authors can see what a plugin expects in its immediate request payload from
  `values.consume`
- authors can see what a plugin actually emits
- future tooling can warn when `baggage:` claims or `with:` mappings refer to
  names not declared by the neighboring plugin contracts
- we avoid inventing a premature type system

## Manifest Shape

```yaml
commands:
  - name: handle
    type: write
    values:
      consume:
        - payload.url
        - payload.message
      emit:
        - event: content_ready
          values:
            - payload.url
            - payload.content
            - payload.content_hash
            - payload.truncated
```

## Rules

- `values.consume` names consumed request payload values.
- `values.emit[].values` names emitted event payload values.
- Current values validation accepts `payload.<path>` and `payload.*` names only.
- `values.emit[].event` is required.
- No JSON types are declared.
- No value becomes durable because it appears in a manifest.
- Authors still use `with:` to transform values into a downstream request.
- Authors still use `baggage:` to name durable facts.

## Code Changes

Changed in the Ductile repo:

```text
internal/plugin/manifest.go
internal/plugin/discovery.go
internal/plugin/discovery_test.go
schemas/plugin-manifest.schema.json
docs/PLUGIN_DEVELOPMENT.md
docs/PIPELINES.md
plugins/fetch/manifest.yaml
plugins/file_handler/manifest.yaml
plugins/sys_exec/manifest.yaml
```

Built-in manifests annotated:

- `fetch`
- `file_handler`
- `sys_exec`
- `echo`
- `file_watch`
- `folder_watch`
- `switch` (legacy compatibility; prefer pipeline `if:` for new authoring)
- `stress`
- `py-greet`
- `ts-bun-greet`

## Verification

```bash
jq empty schemas/plugin-manifest.schema.json
go test ./internal/plugin
go test ./internal/plugin ./internal/router/dsl ./internal/configsnapshot ./internal/inspect
go run ./cmd/ductile config check --config-dir /tmp/ductile-sprint3-batch1-config
go test ./...
git diff --check
```

Results:

- plugin package tests pass
- full Go test suite passes
- copied live config validates with existing duplicate/unused plugin warnings
- JSON schema parses
- diff whitespace check passes

Known caveat:

- `go run ./cmd/ductile config check --config-dir /home/matt/admin/nonprod/ductile-hickey-sprint-3/config` was blocked by `permission denied` reading that non-prod `.checksums` file. The copied live config validation exercises the same manifest parser against live plugin roots.

## Deployment Implication

Do not deploy live config Batch 1 yet.

Before applying Batch 1 to `/home/matt/.config/ductile/pipelines.yaml`, annotate
the relevant external plugin manifests with names-only `values.consume` and
`values.emit`:

- `jina-reader`
- `fabric`
- `astro_rebuild_staging`

`discord_notify` already has the consumed names in legacy `input_schema`
(`message`, `content`, `title`, `username`), but the external-plugin pass
should copy those into `values.consume` so authors have one consistent manifest
surface.

Then re-run the copied-config validation and the non-prod live-shape fixture.
