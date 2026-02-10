---
id: 29
status: done
priority: High
blocked_by: []
assignee: "@codex"
tags: [sprint-2, storage, auth]
---

# Job Storage Enhancement + Auth Middleware

Enhance job storage to persist plugin response payloads and implement API key authentication. Provides data layer for API endpoints.

## Acceptance Criteria

- `job_log` table enhanced with `result` column (JSON blob)
- Plugin response payload stored on job completion
- `GetJobByID()` method returns job with result payload
- API key validation function for auth middleware
- All existing tests still pass
- New tests for enhanced storage and auth

## Implementation Details

**Package:** `internal/queue` (storage) + `internal/api` (auth helper)

### Job Storage Enhancement

**Schema change (internal/storage/sqlite.go):**
```sql
ALTER TABLE job_log ADD COLUMN result TEXT;  -- JSON blob

-- Or if easier, add to bootstrap:
CREATE TABLE IF NOT EXISTS job_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL UNIQUE,
    plugin TEXT NOT NULL,
    command TEXT NOT NULL,
    status TEXT NOT NULL,
    result TEXT,  -- NEW: Plugin response payload
    last_error TEXT,
    stderr TEXT,
    submitted_at DATETIME NOT NULL,
    started_at DATETIME,
    completed_at DATETIME
);
```

**Queue enhancement (internal/queue/queue.go):**
```go
// Enhance Complete() to accept result
func (q *Queue) Complete(
    ctx context.Context,
    jobID string,
    status Status,
    result json.RawMessage,  // NEW: Plugin response payload
    lastError, stderr *string,
) error {
    // Store result in job_log
}

// NEW: Get job by ID for API
type JobResult struct {
    JobID       string
    Status      Status
    Plugin      string
    Command     string
    Result      json.RawMessage
    LastError   *string
    StartedAt   time.Time
    CompletedAt *time.Time
}

func (q *Queue) GetJobByID(ctx context.Context, jobID string) (*JobResult, error) {
    // Query job_log by job_id
    // Return JobResult with all fields
}
```

**Dispatcher integration (internal/dispatch/dispatcher.go):**
- Update call to `Complete()` to pass plugin response
- Currently just passes status/error, now also pass full response

### Authentication

**Auth helper (internal/api/auth.go or keep simple in api/server.go):**
```go
// Simple API key validation
func ValidateAPIKey(providedKey string, configKey string) bool {
    if configKey == "" {
        return false  // API disabled if no key configured
    }
    return providedKey == configKey
}

// Extract from Authorization header
func ExtractAPIKey(r *http.Request) (string, error) {
    auth := r.Header.Get("Authorization")
    if !strings.HasPrefix(auth, "Bearer ") {
        return "", errors.New("missing or invalid Authorization header")
    }
    return strings.TrimPrefix(auth, "Bearer "), nil
}
```

**Note:** Keep it simple for Sprint 2 (single API key). Multiple keys, rotation, etc. can come later.

### Configuration

Add to config types (internal/config/types.go):
```go
type APIConfig struct {
    Enabled bool   `yaml:"enabled"`
    Listen  string `yaml:"listen"`
    Auth    struct {
        APIKey string `yaml:"api_key"`
    } `yaml:"auth"`
}

// Add to Config struct
API APIConfig `yaml:"api"`
```

### Testing

- Test job storage with result payload
- Test GetJobByID() returns correct data
- Test GetJobByID() for non-existent job (returns error)
- Test ValidateAPIKey() with valid/invalid keys
- Test ExtractAPIKey() with valid/missing headers
- Integration test: Complete job with result → GetJobByID returns it

## Interface Provided (to Agent 1)

```go
// Job retrieval
func (q *Queue) GetJobByID(ctx context.Context, jobID string) (*queue.JobResult, error)

// Auth validation
func ValidateAPIKey(providedKey string, configKey string) bool
func ExtractAPIKey(r *http.Request) (string, error)
```

Agent 1 (Claude) uses these in HTTP handlers.

## Branch

`codex/job-storage-auth`

## Merge Order

**Merge BEFORE Agent 1 (claude/api-server)**
Agent 1 depends on this interface. Can develop in parallel with mocks, but merge this first.

## Verification

```bash
# Test job storage
go test ./internal/queue -run TestGetJobByID

# Test auth
go test ./internal/api -run TestValidateAPIKey

# Integration test (after Agent 1 merges)
curl -X POST http://localhost:8080/trigger/echo/poll \
  -H "Authorization: Bearer test-key"
# Should work

curl -X POST http://localhost:8080/trigger/echo/poll \
  -H "Authorization: Bearer wrong-key"
# Should return 401
```

## Narrative

- 2026-02-09: PR #8 submitted. Review complete: Excellent implementation. Added job_log.result column with safe migration for existing databases, implemented GetJobByID() with proper error handling (ErrJobNotFound), added auth helpers (ValidateAPIKey with constant-time comparison, ExtractAPIKey), updated Complete() signature to accept and store plugin response JSON, modified dispatcher to pass results through pipeline, comprehensive test coverage, all tests passing. Provides exact interface needed by Agent 1 (Claude) for API server. Ready to merge. (by @claude)
- 2026-02-09: Codex refactored approach (commit c04df6c): Kept original Complete() signature for backwards compatibility, added new CompleteWithResult() for storing results. Also made JobResult.StartedAt nullable (queued jobs haven't started yet). Cleaner API design. All tests still passing. Approved. (by @claude)