# ductile-dzt spec

## Files likely to touch
- `internal/router/dsl/types.go`
- `internal/router/dsl/compiler.go`
- `internal/router/dsl/compiler_test.go`
- `internal/router/engine.go`
- `internal/router/interface.go`
- `internal/dispatch/dispatcher.go`
- `internal/dispatch/dispatcher_test.go`
- `internal/e2e/pipeline_test.go`
- `schemas/pipelines.schema.json`
- `docs/PIPELINES.md`
- `internal/router/conditions/{types.go,validate.go,eval.go,paths.go,ops.go,errors.go}`
- tests beside the new conditions files

## Expected behaviour
- Pipeline steps may declare structured `if` conditions using either:
  - atomic predicate: `path`, `op`, optional `value`
  - composite predicate: `all`, `any`, or `not`
- Allowed roots for `path`: `payload`, `context`, `config`
- Conditions are validated during pipeline compile/load, not at dispatch execution time
- Runtime condition evaluation happens before plugin spawn
- If condition evaluates false, the step is marked skipped with a visible reason and downstream routing continues as if the step completed successfully for control-flow purposes
- Existing pipelines without `if` remain unchanged
- Existing `switch` plugin remains unchanged

## Edge cases
- Missing path resolves to null for non-`exists` operators and to absent/present for `exists`
- Strict typing only: numeric comparisons require numeric operands; no string-to-number coercion
- Invalid shapes rejected: multiple condition modes set, unknown operator, missing path/op, forbidden value for `exists`, missing value for comparison operators
- Max depth 3 and max predicate count 20 enforced
- Composite conditions short-circuit deterministically
- `call` steps with `if` should skip entry into called pipeline when false
- Skipped steps in multi-step pipelines must continue to successors without spawning a plugin process

## Test plan
- Unit tests for `internal/router/conditions` covering path resolution, operators, strict typing, composite logic, short-circuiting, and validation limits
- DSL/compiler tests for valid/invalid `if` definitions and backward compatibility without `if`
- Dispatcher/router integration tests proving `if=true` executes, `if=false` skips and continues, mixed run/skip pipelines behave correctly, and skipped reason/status are persisted observably
- E2E smoke test with real temp DB/workspace/plugins proving observable results and terminal statuses for an `if` pipeline
