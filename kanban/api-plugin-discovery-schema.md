---
id: 107
status: doing
priority: High
blocked_by: []
assignee: "@gemini"
tags: [api, plugins, schema, rfc-004]
---

# API: Plugin Discovery and Schema Export

Implement API endpoints to allow agents to list available plugins and retrieve their metadata, including optional input/output schemas.

## Job Story
When I am an LLM agent operating the gateway, I want to list available plugins and their schemas via API, so I can understand the system's capabilities and how to invoke them correctly.

## Acceptance Criteria
- [ ] New endpoint `GET /skills` (or `/plugins`) returning a list of available plugins and their metadata.
- [ ] New endpoint `GET /plugin/{name}` returning full metadata for a specific plugin.
- [ ] Manifest format supports optional `input_schema` and `output_schema` (JSON Schema).
- [ ] Endpoints respect `plugin:ro` or `plugin:rw` scopes.
- [ ] Documentation updated in `API_REFERENCE.md`.

## Narrative
- 2026-02-18: Created card to implement plugin discovery and schema export as requested by user. (by @gemini)
