---
id: 120
status: todo
priority: Normal
blocked_by: []
assignee: ""
tags: [documentation, v2.0, api, config]
---

# #120: v2.0 Documentation Uplift

Update the gateway documentation to reflect major architectural changes in v2.0, including multi-root discovery, separate trigger endpoints, and the `uv` migration.

## Goal

Ensure that `docs/` reflect the current reality of the codebase after the v2.0 feature push.

## Areas to Update

### 1. Multi-Root Discovery
- Update `docs/CONFIG_REFERENCE.md` to document `plugin_roots` and the deprecation of `plugins_dir`.
- Update `docs/GETTING_STARTED.md` with examples of mounting external plugin volumes.
- Update `docs/ARCHITECTURE.md` to show the new discovery flow.

### 2. API Endpoints
- Update `docs/API_REFERENCE.md` to prominently feature `/plugin/{p}/{c}` and `/pipeline/{n}`.
- Mark `/trigger/{p}/{c}` as deprecated in the docs.
- Document unauthenticated discovery endpoints (`/plugins`, `/skills`, `/plugin/{name}/openapi.json`).

### 3. Plugin Development (uv)
- Update `docs/PLUGIN_DEVELOPMENT.md` to recommend `uv` for Python plugins.
- Add an example of PEP 723 inline script metadata for dependencies.
- Remove old `requirements.txt` based instructions.

### 4. Event Context Lineage
- Update `docs/PIPELINES.md` to explain how `EventContext` is managed when triggering via API vs event-driven routing.

## Acceptance Criteria
- [ ] `docs/CONFIG_REFERENCE.md` covers `plugin_roots`.
- [ ] `docs/API_REFERENCE.md` covers new endpoints and deprecations.
- [ ] `docs/PLUGIN_DEVELOPMENT.md` features `uv` as the primary tool.
- [ ] All examples use the new v2.0 conventions.

## Narrative
- 2026-02-22: Card created to track documentation debt after the v2.0 feature merge. (by @assistant)
