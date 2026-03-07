# ductile-j3x spec

## Files likely to touch
- `internal/dispatch/dispatcher.go`
- `internal/api/integration_test.go`
- maybe `internal/dispatch/dispatcher_test.go`

## Expected behaviour
- Synchronous API waiting should not return an incomplete job tree when the root step is skipped by first-class if and downstream children already exist or are about to become visible.
- Wait logic should require a stable/settled complete tree before returning.
- Comments should explain why the extra confirmation exists.

## Edge cases
- root skipped step with one child
- root skipped step with multiple children
- ordinary trees should not regress significantly
- avoid introducing long sleeps or brittle timing assumptions

## Test plan
- use existing API integration proof test and update it from proof-of-bug to regression test
- rerun dispatch/api/e2e tests
- rerun Docker functional test for false/true branches
