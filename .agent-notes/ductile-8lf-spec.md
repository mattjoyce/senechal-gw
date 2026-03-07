# ductile-8lf spec

## Files likely to touch
- `internal/dispatch/dispatcher.go`
- `internal/api/handlers.go`
- `internal/dispatch/dispatcher_test.go`
- `internal/api/integration_test.go` or `internal/e2e/pipeline_test.go`

## Expected behaviour
- When a root API-triggered pipeline starts at a step with `if=false`, Ductile should still continue to downstream successor steps.
- Synchronous waiting should not finish early just because the skipped root job has reached a terminal state before child jobs are enqueued.
- Returned job tree should include both the skipped step and any downstream executed steps.

## Edge cases
- root step skipped with one successor
- root step skipped with fan-out successors
- synchronous waiting race between root completion notification and child enqueue
- skip continuation should still preserve terminal status/reason for skipped node

## Test plan
- add focused integration test for synchronous root-triggered pipeline with skipped entry step
- preserve existing dispatcher skip test
- rerun docker functional test after fix
