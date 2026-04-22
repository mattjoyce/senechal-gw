# Docker Test Fixtures

This directory contains fixture-driven Docker/system test scenarios for the staged testing harness.

Current fixtures:
- `webhook-ingress`
- `scheduler-recovery`
- `api-e2e`
- `hook-route-compilation`
- `sync-terminal-route`
- `conditional-with-route`

Current status:
- harness base is scaffolded
- fixture execution wiring exists
- existing fixtures cover webhook ingress, scheduler recovery, API e2e, plugin/runtime behavior, and workspace behavior
- Sprint 5 adds route-runtime regression fixtures for hook-entry `call:` expansion and synchronous terminal result selection
- Sprint 6 adds a route-runtime regression fixture for compiled `if:` branching plus `with:` remapping on the true branch
