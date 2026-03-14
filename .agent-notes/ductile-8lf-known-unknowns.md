# ductile-8lf known / unknowns

## Known
- The original Docker functional test exposed a real race/ordering issue around skipped entry steps in synchronous root pipelines.
- Completing the skipped root job before routing successors allowed the synchronous waiter/API response to observe the tree too early.
- Reordering skip handling to persist the skipped terminal state and then attempt successor routing avoids marking the skip as a hard failure when workspace cloning is unavailable, but does not by itself make the synchronous API tree include downstream jobs.
- Docker retest still shows:
  - `if=false` => synchronous response contains only the skipped root step
  - `if=true` => synchronous response contains both executed steps
- Existing dispatcher/e2e tests pass, so the remaining gap is specific to API-triggered synchronous root execution timing and/or waiter semantics.

## Unknown
- Whether the core issue is primarily in `WaitForJobTree` notification timing, API synchronous response assembly, or root-trigger enqueue/context semantics.
- Whether root skipped-step continuation actually occurs after the API response returns, or is not happening at all in the root path under Docker.
- Whether a robust fix should:
  - delay waiter completion until routing successor enqueue is definitely visible, or
  - explicitly create/track expected successor jobs before marking skipped root terminal, or
  - adjust synchronous API semantics for skipped entry nodes.

## Most likely next investigation points
- instrument / inspect queue state immediately after the `if=false` sync API response in Docker
- inspect whether child job rows exist but are missed by waiter timing
- add a focused API-level integration test for synchronous skipped-entry pipeline that asserts downstream child presence in returned tree
