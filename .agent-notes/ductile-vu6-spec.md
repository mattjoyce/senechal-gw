# ductile-vu6 spec

## Files likely to touch
- `internal/dispatch/dispatcher.go`
- `internal/dispatch/dispatcher_test.go`
- maybe `internal/e2e/pipeline_test.go`

## Expected behaviour
- Dispatcher has a small explicit preflight phase before plugin spawn
- Preflight ensures workspace exists before if evaluation
- Preflight returns one of: run, skip, fail
- Skip outcome preserves current observable behaviour (`skipped` status + reason)
- Workspace inheritance for skipped steps becomes valid because skipped jobs now have a workspace
- Keep abstraction minimal; no broad state machine

## Edge cases
- jobs with no workspace manager configured
- jobs with missing context
- steps without if conditions
- skipped root steps with child workspace clone

## Test plan
- update/extend dispatcher tests for skipped step continuation with workspace manager
- rerun targeted API and docker functional tests
