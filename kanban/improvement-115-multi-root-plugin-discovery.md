---
id: 115
status: backlog
priority: Normal
blocked_by: []
tags: [plugins, config, discovery, security, architecture]
---

# #115: Multi-Root Plugin Discovery for External Plugin Repos

Support plugin discovery from multiple filesystem roots so plugins can live outside the `ductile` repo (for example, a shared `ductile-plugins` repo and/or separate private plugin repos).

## Job Story
When plugins are managed in separate repositories, I want Ductile to scan multiple configured plugin roots, so I can deploy and version plugin code independently from the core gateway.

## Proposed Config
```yaml
plugin_roots:
  - /opt/ductile-plugins
  - /srv/ductile-private-plugins

plugins:
  echo:
    enabled: true
```

## Acceptance Criteria
- [ ] Add `plugin_roots` (array of directories) to config schema and validation.
- [ ] Keep `plugins_dir` as a backward-compatible fallback during migration.
- [ ] Update plugin discovery to scan all configured roots and load plugins with valid `manifest.yaml`.
- [ ] Maintain trust checks: entrypoint must resolve under an approved root and under the plugin directory; keep path traversal and world-writable directory protections.
- [ ] Define deterministic duplicate plugin name behavior across roots (document precedence + warning/error behavior).
- [ ] Manifest schema supports declaring required environment variable names for secrets/credentials (references only).
- [ ] Secret values are never stored in `manifest.yaml` (load from runtime environment and/or configured env file).
- [ ] Update operator docs and getting-started docs with external plugin repo examples.
- [ ] Update/expand tests for multi-root discovery, duplicate-name handling, and trust validation across roots.

## Notes
- Avoid naming the new field `plugins` to prevent collision with the existing runtime plugin config map.
- Prefer `plugin_roots` to make intent explicit.
- Model A: one shared `ductile-plugins` repository.
- Model B: individual plugin repositories mounted into approved roots.
- Credentials model: manifest declares required env names (e.g., `GOOGLE_API_KEY`), while values come from process env or optional `env_file`.

## Narrative
- 2026-02-21: Created card from architecture discussion about decoupling plugin code from the core repo via multi-root plugin discovery. (by @codex)
- 2026-02-21: Added security/config decision: manifests may declare env-var references for secrets but must not embed secret values. (by @codex)
