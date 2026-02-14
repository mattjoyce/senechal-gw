---
id: 65
status: done
priority: High
blocked_by: []
tags: [bug, database, concurrency, performance, core]
---

# BUG: SQLite Database Locking Under Concurrent API Requests

## Description

When multiple API requests attempt to enqueue jobs concurrently, SQLite database locks occur, causing job enqueue failures. This limits the API's ability to handle concurrent workloads.

## Impact

- **Severity**: High
- **User Experience**: API requests fail with 500 errors under concurrent load
- **Workaround**: Client-side retry logic or rate limiting

## Evidence

```
{"time":"2026-02-11T22:03:14Z","level":"ERROR","msg":"failed to enqueue job",
 "error":"enqueue job: database is locked (5) (SQLITE_BUSY)"}
```

## Reproduction

```bash
# Submit 5 concurrent requests
for i in {1..5}; do
  curl -X POST http://localhost:8080/trigger/echo/poll \
    -H "Authorization: Bearer $TOKEN" &
done
wait
```

**Result**: 4/5 requests fail with "database is locked" error

## Root Cause

SQLite by default uses file-based locking and doesn't handle high concurrent write loads well. The issue occurs when:
1. Multiple API handlers try to write to the jobs table simultaneously
2. SQLite locks the database file for exclusive write access
3. Subsequent write attempts receive SQLITE_BUSY error

## Analysis

This is a **core codebase issue** related to the database layer implementation. SQLite is excellent for single-writer scenarios but has limitations with concurrent writes.

## Potential Solutions (for dev team)

1. **Immediate**: Implement retry logic with backoff for SQLITE_BUSY errors
2. **Short-term**: Configure SQLite with WAL (Write-Ahead Logging) mode for better concurrency
3. **Medium-term**: Add connection pooling with proper timeout handling
4. **Long-term**: Consider PostgreSQL for production deployments with high concurrency needs

## Configuration Note

SQLite WAL mode can be enabled with:
```sql
PRAGMA journal_mode=WAL;
```

This allows concurrent readers with a writer, improving throughput significantly.

## Related

- SQLite documentation: https://www.sqlite.org/wal.html
- Known SQLite limitation: https://www.sqlite.org/lockingv3.html

## Test Environment

- **Scenario**: 5 concurrent POST /trigger/echo/poll requests
- **Success Rate**: 20% (1/5)
- **Failure Mode**: SQLITE_BUSY error
- **Database**: SQLite 3.51.2

## Narrative

- 2026-02-12: Discovered during concurrent API testing. When submitting 5 simultaneous job trigger requests, 4 fail with SQLITE_BUSY errors. This is a known limitation of SQLite's default locking behavior. The gateway is using SQLite in the default journal mode which only supports one writer at a time. Under concurrent API load, this causes contention and failures. This is a core codebase issue requiring database layer modifications. Workarounds include client-side retry logic or enabling WAL mode in SQLite. (by @assistant/test-admin)
