# Reload Command Root Cause Analysis

**Date:** 2026-04-12  
**Issue:** `ductile system reload` hangs and eventually fails  
**Status:** Fix updated; live reload smoke test pending

---

## Executive Summary

The `system reload` command was hanging indefinitely and eventually crashing with a panic. The root cause was a self-shutdown deadlock in the API reload handler, plus unsynchronised reload attempts that could stop the same runtime more than once.

---

## Symptoms

1. `curl -X POST http://127.0.0.1:8082/system/reload` hangs for ~10 seconds
2. Returns empty response (curl code 52)
3. Server process remains alive but becomes unreachable
4. Eventually logs show "panic: close of closed channel"

---

## Root Causes

### 1. API Server Shutdown Deadlock

**Location:** `internal/api/server.go:133-140`

```go
// Shutdown() waits for in-flight requests to complete
shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
if err := s.server.Shutdown(shutdownCtx); err != nil {
    return fmt.Errorf("server shutdown failed: %w", err)
}
```

The old API server's `Shutdown()` waits for in-flight requests. But the reload request was itself waiting for the old runtime shutdown to complete, creating a circular wait.

### 2. Port Binding Race Condition

The new runtime tried to bind to the same port before the old server released it. A fixed sleep could make this less likely, but it did not make the handoff deterministic.

### 3. Repeated Runtime Stop

```go
func (s *Scheduler) Stop() {
    close(s.stopCh)
    s.wg.Wait()
}
```

Reload only locked around reading and writing the runtime pointer. Two concurrent reloads, or a reload racing with shutdown, could call `Stop()` on the same runtime twice and panic on the second scheduler channel close.

---

## Fixes Applied

### Fix 1: Detached Context

**File:** `internal/api/handlers.go`

```go
// AFTER (fixed)
resp, err := s.reloadFunc(context.Background())
```

Using `context.Background()` prevents client disconnects from cancelling a protected reload after it starts.

### Fix 2: Serialized, Idempotent Runtime Stop

**File:** `cmd/ductile/main.go`

```go
rm.mu.Lock()
defer rm.mu.Unlock()

rt.stopOnce.Do(func() {
    defer close(rt.stopDone)
    rt.cancel()
    rt.scheduler.Stop()
    rt.wg.Wait()
})
<-rt.stopDone
```

The reload manager now serializes the full reload lifecycle, and runtime stop is idempotent. That prevents concurrent reloads or shutdown races from stopping the same scheduler twice.

### Fix 3: Deterministic Listener Handoff

**Files:** `internal/api/server.go`, `internal/webhook/server.go`, `cmd/ductile/main.go`

```go
go oldRuntime.Stop()

listenerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
defer cancel()
if err := oldRuntime.WaitListenersStopped(listenerCtx); err != nil {
    return api.ReloadResponse{Status: "error", Message: err.Error()}, err
}

runtime, err := buildRuntime(newCfg, rm.configPath, rm.configSource, rm.reloadFunc, rm.errCh)
```

The old runtime still shuts down in the background so the self-initiating reload request can return, but the replacement runtime is not built until the old API and webhook serve loops have stopped. This removes the fixed sleep from the port handoff.

---

## Sequence Diagram

```
Client                  API Server              Reload Manager
  |                        |                        |
  |--- POST /reload ------>|                        |
  |                        |--- reloadFunc() ----->|
  |                        |                        |
  |                        |            [loads new config]
  |                        |            [validates integrity]
  |                        |                        |
  |                        |            [go oldRuntime.Stop()]
  |                        |            [wait for listeners stopped]
  |                        |                        |
  |                        |            [build new runtime]
  |                        |                        |
  |                        |            [swap runtimes]
  |                        |                        |
  |                        |<-- success -----------|
  |<-- 200 OK -------------|                        |
  |                        |                        |
```

---

## Verification

Automated verification:

```bash
go test ./internal/api ./internal/webhook
go test ./cmd/ductile -run TestRuntimeStateStopIsIdempotent -count=1
go test -race ./cmd/ductile -run TestRuntimeStateStopIsIdempotent -count=1
go test -race ./internal/api -run TestServerWaitServeStoppedAfterCancel -count=1
```

Pending live smoke verification:

```bash
curl -X POST http://127.0.0.1:8082/system/reload \
  -H "Authorization: Bearer <token>"
```

---

## Files Modified

| File | Change |
|------|--------|
| `internal/api/handlers.go` | Detach reload from client disconnects |
| `internal/api/server.go` | Expose serve-loop completion |
| `internal/webhook/server.go` | Expose serve-loop completion |
| `cmd/ductile/main.go` | Serialize reload, make runtime stop idempotent, wait for listener handoff |

---

## Lessons Learned

1. **Do not bind reload to client disconnects**: Once a protected reload begins, it should either complete or fail explicitly.

2. **Graceful shutdown must not wait for itself**: If a handler initiates shutdown, it cannot wait for shutdown to complete.

3. **Port handoff needs a real signal**: Fixed sleeps reduce failure probability but do not establish correctness.

4. **Stop paths must be idempotent**: Shutdown and reload can race with each other; stopping an already-stopping runtime must be safe.

---

## Related Configuration

The reload command validates these constraints:

- Config must be locked: `ductile config lock`
- Cannot change: `state.path`, `api.listen`, `webhooks.listen`

See `validateReloadableFields()` in `cmd/ductile/main.go` for implementation.
