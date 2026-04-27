# Docker Test Fixtures

This directory contains fixture-driven Docker/system test scenarios for the staged testing harness.

Current fixtures:
- `webhook-ingress`
- `scheduler-recovery`
- `api-e2e`
- `file_watch`
- `hook-route-compilation`
- `sync-terminal-route`
- `conditional-with-route`
- `pipeline-level-if`

Current status:
- harness base is scaffolded
- fixture execution wiring exists
- existing fixtures cover webhook ingress, scheduler recovery, API e2e, and plugin/runtime behavior
- Sprint 5 adds route-runtime regression fixtures for hook-entry `call:` expansion and synchronous terminal result selection
- Sprint 6 adds a route-runtime regression fixture for compiled `if:` branching plus `with:` remapping on the true branch
- Sprint 7 extends `file_watch` to prove append-only `plugin_facts`, derived compatibility state, and operator inspection via `ductile system plugin-facts`
- Hickey Sprint 16 adds `pipeline-level-if` covering the new trigger-level `if:` predicate end-to-end across `on:` and `on-hook:` paths
