// Package dispatch handles plugin subprocess spawning, protocol v1 I/O, and timeout enforcement.
//
// The dispatcher dequeues jobs from the queue and executes them by spawning plugin
// subprocesses according to the protocol v1 specification. It enforces timeouts,
// captures stderr, updates plugin state, and handles job completion.
//
// Key features:
//   - Serial FIFO dispatch (one job at a time)
//   - Spawn-per-command subprocess execution
//   - Timeout enforcement with SIGTERM → 5s grace → SIGKILL
//   - Protocol v1 JSON over stdin/stdout
//   - Workspace/context injection for Governance Hybrid orchestration
//   - State persistence via shallow merge
//   - Stderr capture (capped at 64KB)
//   - Event routing + downstream enqueueing
//
// Timeout handling:
//   - Each command has a configured timeout (from config.TimeoutsConfig)
//   - When timeout expires, SIGTERM is sent to the plugin process
//   - After 5 second grace period, SIGKILL is sent if process still running
//   - Job is marked as timed_out with error message
//
// Error handling:
//   - Plugin not found → failed status
//   - Unsupported command → failed status
//   - Protocol error (invalid JSON) → failed status
//   - Plugin returns error status → failed status
//   - Timeout → timed_out status
//   - Success → succeeded status with state updates applied
//
// MVP limitations:
//   - Retry logic not implemented (jobs fail permanently)
//   - No circuit breaker enforcement
package dispatch
