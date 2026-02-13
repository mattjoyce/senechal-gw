# Ductile Test Results

**Test Date**: 2026-02-12
**Tester**: System Admin / QA
**Environment**: Docker test container (~/ductile/)
**Gateway Version**: Latest from main branch

---

## Executive Summary

Completed systematic testing of ductile in Docker test environment. Found and fixed 2 plugin issues. Identified 1 core database concurrency issue requiring dev team attention.

### Overall Status: ✅ FUNCTIONAL with Known Limitations

- **Core Functionality**: Working
- **API Server**: Operational
- **Authentication**: Secure and correct
- **Plugin System**: Working (after fixes)
- **Scheduler**: Operational
- **Known Issues**: 1 High priority (SQLite concurrency)

---

## Test Results Summary

| Category | Tests | Pass | Fail | Notes |
|----------|-------|------|------|-------|
| Plugin System | 5 | 5 | 0 | Fixed 2 manifest/implementation bugs |
| API Server | 8 | 8 | 0 | All auth scenarios work correctly |
| Scheduler | 2 | 2 | 0 | Basic scheduling operational |
| State Management | 3 | 3 | 0 | SQLite persistence working |
| Job Processing | 4 | 4 | 0 | Queue, execution, status working |
| Concurrency | 1 | 0 | 1 | SQLite locking under concurrent writes |
| Configuration | 2 | 2 | 0 | Config loading and env vars work |
| Error Handling | 4 | 4 | 0 | Proper error responses |
| Logging | 2 | 2 | 0 | JSON structured logging working |
| Container/Docker | 4 | 4 | 0 | Build, startup, health checks pass |

**Total**: 35 tests, 34 passed, 1 failed

---

## Detailed Test Results

### 1. Plugin System ✅

#### Test 1.1: Plugin Discovery
- **Status**: ✅ PASS
- **Result**: Gateway discovers plugins in ./plugins/ directory
- **Plugins Found**: echo (v0.1.0), fabric (v0.1.0)

#### Test 1.2: Plugin Manifest Validation
- **Status**: ✅ PASS (after fix)
- **Issue Found**: Fabric manifest had invalid command structure
- **Fix Applied**: Corrected manifest format
- **Card**: #59 (Done)

#### Test 1.3: Plugin Execution (poll)
- **Status**: ✅ PASS
- **Result**: Both echo and fabric plugins execute poll command successfully
- **Evidence**: Jobs complete with status "succeeded"

#### Test 1.4: Plugin Error Handling
- **Status**: ✅ PASS
- **Test**: Trigger non-existent plugin
- **Result**: Proper 400 error: "plugin not found"

#### Test 1.5: Invalid Command Handling
- **Status**: ✅ PASS
- **Test**: Trigger echo plugin with invalid command
- **Result**: Proper 400 error: "command not supported by plugin"

**Issues Fixed**:
- #59: Fabric plugin manifest format (DONE)
- #62: Fabric plugin command mismatch (DONE)

---

### 2. API Server ✅

#### Test 2.1: No Authentication
- **Status**: ✅ PASS
- **Request**: POST /trigger/echo/poll (no auth)
- **Result**: 401 Unauthorized - "missing Authorization header"

#### Test 2.2: Invalid Token
- **Status**: ✅ PASS
- **Request**: POST with invalid Bearer token
- **Result**: 401 Unauthorized - "invalid API key"

#### Test 2.3: Valid Admin Token
- **Status**: ✅ PASS
- **Request**: POST with valid admin token
- **Result**: 202 Accepted - Job queued successfully

#### Test 2.4: Insufficient Scope
- **Status**: ✅ PASS
- **Request**: POST with read-only token (trying to trigger)
- **Result**: 403 Forbidden - "insufficient scope"

#### Test 2.5: Job Status Retrieval
- **Status**: ✅ PASS
- **Test**: GET /job/{job_id}
- **Result**: Returns correct job status and result

#### Test 2.6: Invalid Job ID
- **Status**: ✅ PASS
- **Test**: GET /job/invalid-uuid
- **Result**: 404 Not Found - "job not found"

#### Test 2.7: Non-existent Job
- **Status**: ✅ PASS
- **Test**: GET /job/00000000-0000-0000-0000-000000000000
- **Result**: 404 Not Found - "job not found"

#### Test 2.8: API Response Format
- **Status**: ✅ PASS
- **Result**: All responses are valid JSON with proper structure

**Card**: #60 (Done) - API Authentication Test Results

---

### 3. Scheduler ✅

#### Test 3.1: Basic Scheduling
- **Status**: ✅ PASS
- **Config**: Echo plugin scheduled every 5m
- **Result**: Jobs enqueued automatically at correct intervals
- **Evidence**: Logs show "Enqueued poll job" with dedupe_key

#### Test 3.2: Plugin State After Scheduled Execution
- **Status**: ✅ PASS
- **Result**: State updates persist (last_run, job_id)

---

### 4. State Management ✅

#### Test 4.1: Database Initialization
- **Status**: ✅ PASS
- **Result**: SQLite database created at ./data/ductile-test.db

#### Test 4.2: State Persistence
- **Status**: ✅ PASS
- **Test**: Job state persists across executions
- **Result**: last_run and job_id stored correctly

#### Test 4.3: State Retrieval
- **Status**: ✅ PASS
- **Result**: Plugin receives current state in request envelope

---

### 5. Job Processing ✅

#### Test 5.1: Job Queueing
- **Status**: ✅ PASS
- **Result**: Jobs transition: queued → running → succeeded

#### Test 5.2: Job Execution
- **Status**: ✅ PASS
- **Result**: Plugins execute with correct timeout (60s default)

#### Test 5.3: Job Status Tracking
- **Status**: ✅ PASS
- **Result**: started_at, completed_at timestamps recorded

#### Test 5.4: Job Result Capture
- **Status**: ✅ PASS
- **Result**: Plugin output (status, logs, state_updates) captured correctly

---

### 6. Concurrency ❌

#### Test 6.1: Concurrent Job Submissions
- **Status**: ❌ FAIL
- **Test**: 5 simultaneous POST /trigger/echo/poll requests
- **Result**: 1/5 succeeded, 4/5 failed
- **Error**: "database is locked (5) (SQLITE_BUSY)"
- **Impact**: High - Limits API throughput under concurrent load
- **Card**: #61 (TODO) - SQLite Concurrent Write Locking

**Root Cause**: SQLite default locking behavior doesn't support concurrent writes well

**Recommended Fix**: Enable WAL mode or implement retry logic (core team)

---

### 7. Configuration ✅

#### Test 7.1: Config File Loading
- **Status**: ✅ PASS
- **Result**: config.test.yaml loaded successfully

#### Test 7.2: Environment Variable Interpolation
- **Status**: ✅ PASS
- **Test**: ${DUCTILE_TOKEN_ADMIN} and other vars
- **Result**: Variables correctly substituted from .env.test

---

### 8. Error Handling ✅

#### Test 8.1: Plugin Errors
- **Status**: ✅ PASS
- **Test**: Plugin returns error status
- **Result**: Job status = "failed", error message captured

#### Test 8.2: Invalid Requests
- **Status**: ✅ PASS
- **Result**: Proper 400/401/403/404 responses

#### Test 8.3: Missing Required Fields
- **Status**: ✅ PASS
- **Result**: Appropriate error messages

#### Test 8.4: Network Errors
- **Status**: ✅ PASS (implicit)
- **Result**: Container handles port binding correctly

---

### 9. Logging ✅

#### Test 9.1: Structured Logging
- **Status**: ✅ PASS
- **Format**: JSON with timestamp, level, msg, component
- **Result**: Easy to parse and filter

#### Test 9.2: Log Levels
- **Status**: ✅ PASS
- **Config**: DEBUG level enabled for testing
- **Result**: DEBUG, INFO, WARN, ERROR logs visible

---

### 10. Container/Docker ✅

#### Test 10.1: Build Process
- **Status**: ✅ PASS
- **Time**: ~50 seconds
- **Image Size**: 129MB

#### Test 10.2: Container Startup
- **Status**: ✅ PASS
- **Startup Time**: ~3 seconds
- **Result**: No errors in startup logs

#### Test 10.3: Health Check
- **Status**: ✅ PASS
- **Method**: PID file existence check
- **Result**: Container marked healthy

#### Test 10.4: Resource Usage
- **Status**: ✅ PASS
- **CPU**: 0.23%
- **Memory**: 4.7 MB / 15.25 GB (0.03%)
- **Assessment**: Very efficient

---

## Issues Found

### Critical/High Priority

1. **#61 - SQLite Concurrent Write Locking** (TODO, High)
   - **Impact**: API fails under concurrent load
   - **Affected**: Core database layer
   - **Recommendation**: Enable WAL mode or implement retry logic

### Fixed During Testing

2. **#59 - Fabric Plugin Manifest Format** (DONE)
   - **Issue**: Invalid manifest structure
   - **Fix**: Corrected to simple list format
   - **Status**: Fixed and verified

3. **#62 - Fabric Plugin Command Mismatch** (DONE)
   - **Issue**: Implementation used "execute" instead of "poll"
   - **Fix**: Refactored to implement poll/handle/health
   - **Status**: Fixed and verified

### Documentation

4. **#60 - API Authentication Test Results** (DONE)
   - **Type**: Test documentation
   - **Status**: Comprehensive auth testing documented

---

## Not Tested (Requires Dev Team Implementation)

- Crash recovery mechanism
- Circuit breaker functionality
- Job retry logic with max attempts
- Config reload (SIGHUP)
- Webhook endpoints (not enabled)
- Event routing (not configured)
- Plugin handle command with events
- Plugin health command
- Plugin init command
- State size limit (1MB) enforcement
- Job log retention pruning

---

## Recommendations

### Immediate (P0)
1. ✅ Fix plugin manifest issues - **DONE**
2. 🔴 Address SQLite concurrency (#61) - **TODO** (dev team)

### Short-term (P1)
3. Test crash recovery behavior
4. Test circuit breaker with failing plugin
5. Add integration tests for concurrent scenarios
6. Document fabric plugin usage (handle command)

### Medium-term (P2)
7. Performance testing under sustained load
8. Test with multiple plugins scheduled simultaneously
9. Verify state size limits
10. Test job retention and pruning

---

## Test Environment Details

- **Location**: ~/ductile/
- **Container**: ductile-test
- **Config**: config.test.yaml
- **Database**: ./data/ductile-test.db
- **API**: http://localhost:8080
- **Logging**: DEBUG level enabled

---

## Conclusion

The Ductile core functionality is **solid and working well**. The plugin system, API server, authentication, and basic job processing all work correctly. The main limitation is SQLite's concurrent write handling, which is a known trade-off of using SQLite.

**Ready for**: Development team testing with proper awareness of SQLite limitations

**Blockers**: None critical, but concurrent workload testing should account for SQLite behavior

**Next Steps**:
1. Dev team to review #61 (SQLite concurrency)
2. Continue testing advanced features (webhooks, routing, circuit breakers)
3. Performance testing under realistic workloads
