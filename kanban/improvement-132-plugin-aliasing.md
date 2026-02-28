---
id: 132
status: todo
priority: Normal
blocked_by: []
tags: [improvement, plugins, config]
---

# Plugin Aliasing / Instance Names

## Job Story

When I want multiple differently-configured instances of the same plugin binary, I want a config-level alias mechanism so I don’t have to duplicate plugin folders/manifests just to change configuration.

## Problem

Today, plugin names must match manifest names. Multiple “instances” require copying the plugin directory and manifest, which is error-prone and hard to maintain.

## Proposed Direction

Add a `uses:` field under `plugins.<instance>` to map an instance name to a base plugin manifest:

```yaml
plugins:
  check_youtube:
    uses: switch
    config: { ... }

  check_status:
    uses: switch
    config: { ... }
```

## Implementation Sketch

- **Config load:** keep `plugins.<instance>` as-is, but allow `uses` to reference a discovered manifest.
- **Registry view:** expose *instance* names to the dispatcher/API/TUI; each instance points to a base manifest + instance config overrides.
- **Manifest reuse:** base manifest is loaded once; instance config is merged on dispatch (same binary, different config).
- **Optional guard:** if `plugins.<instance>.uses` is set, ignore/override `manifest.name` conflicts (instance name wins).

## Notes / Considerations

- Dispatcher resolves instance name → base plugin manifest + instance config.
- `doctor` validates `uses:` targets exist.
- API/CLI/TUI list instance names (not just base manifests).
- Decide whether auth scopes apply to instance names (recommended) vs base plugin names.
- Migration: document replacing copied plugin folders (e.g., `check_youtube` → `switch` + `uses`).

## Acceptance Criteria

- [ ] Config supports `plugins.<instance>.uses: <plugin_name>`.
- [ ] Multiple instances can reference the same manifest.
- [ ] Instance config overrides are applied correctly.
- [ ] `ductile status` and `/plugins` list instance names.
- [ ] `doctor` validates missing `uses` targets.
- [ ] Token scopes work with instance names (documented).

## Narrative

- 2026-02-27: Card created to avoid duplicating plugin folders for multiple configs. (by @assistant)
