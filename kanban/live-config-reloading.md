---
id: 106
status: todo
priority: Normal
blocked_by: []
assignee: "@gemini"
tags: [core, config, lifecycle]
---

# #106: Live Configuration Reloading

Implement a mechanism to reload the gateway's configuration while it is running, without requiring a full process restart.

## Job Story
When I update the YAML configuration files, I want the gateway to pick up the changes immediately (or via a signal/API call), so I can iterate on pipelines and plugin settings without interrupting the service.

## Acceptance Criteria
- [ ] Implement a `SIGHUP` signal handler to trigger a configuration reload.
- [ ] (Optional) Implement an administrative API endpoint `POST /api/admin/reload` to trigger a reload.
- [ ] The reload process should:
    - Re-validate the configuration on disk.
    - Refresh the plugin registry (discover new/removed plugins).
    - Refresh the router/pipelines.
    - Update scheduler tick intervals if they changed.
- [ ] Ensure the reload is thread-safe and doesn't crash active jobs.
- [ ] Log success/failure of the reload operation.

## Narrative
- 2026-02-16: Created card #106 to address the need for live configuration reloading as requested by the user. (by @gemini)
