# Docker Test Fixtures

This directory contains fixture-driven Docker/system test scenarios for the staged testing harness.

Current fixtures:
- `webhook-ingress`
- `scheduler-recovery`
- `api-e2e`

Current status:
- harness base is scaffolded
- fixture execution wiring exists
- `webhook-ingress` fixture is partially implemented and has uncovered a follow-up config/runtime issue tracked in `ductile-pyn`
- `scheduler-recovery` and `api-e2e` are placeholders pending implementation
