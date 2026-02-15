# Testing Guide

This document outlines the testing strategy and procedures for Ductile, including manual E2E validation, automated CLI testing, and API endpoint verification.

---

## 1. Automated Tests

Ductile uses standard Go testing patterns.

### Unit & Integration Tests
Run all internal tests:
```bash
go test ./...
```

### CLI Test Suite
Comprehensive testing of CLI actions, exit codes, and output formats:
```bash
# See docs/CLI_DESIGN_PRINCIPLES.md for the standards being tested
go test ./cmd/ductile/...
```

---

## 2. Manual E2E Validation (The Echo Runbook)

To verify the full "Trigger to Output" lifecycle, use the `echo` plugin.

1.  **Configure:** Set a 5-minute schedule for `echo` in `config.yaml`.
2.  **Start:** `./ductile system start`.
3.  **Verify:** Check the logs or query the state DB:
    ```bash
    sqlite3 ductile.db "SELECT * FROM plugin_state WHERE plugin_name = 'echo';"
    ```
4.  **Crash Recovery:** Force-kill the process (`kill -9`) while a job is running, then restart and verify the job is recovered.

---

## 3. Pipeline Testing

Verify multi-hop event chains using the `file-to-report` example:

1.  **Trigger:**
    ```bash
    curl -X POST http://localhost:8080/trigger/file_handler/handle \
      -H "Authorization: Bearer <token>" \
      -d '{"payload": {"action": "read", "file_path": "sample.md"}}'
    ```
2.  **Trace:** Use the `inspect` tool to view the lineage:
    ```bash
    ductile job inspect <job_id>
    ```
3.  **Artifacts:** Verify the workspace directory contains the hardlinked files from previous steps.

---

## 4. Test Environment

We provide a **Docker-based** environment for reproducible testing:
```bash
cd ductile
docker compose up --build
```
See `TEST_ENVIRONMENT.md` for details on pre-configured tokens and ports.

---

## 5. API Endpoint Testing

As of PR #43, Ductile provides three HTTP endpoints with distinct semantics:

### 5.1 Direct Plugin Execution (`/plugin/{plugin}/{command}`)

**Purpose:** Execute a plugin directly without pipeline routing.

**Test Case: Jina-Reader Scrape**
```bash
curl -X POST http://localhost:8080/plugin/jina-reader/handle \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "payload": {
      "url": "https://github.com/mattjoyce?tab=repositories"
    }
  }'
```

**Expected Result:**
- Response contains jina-reader output only
- No pipeline execution (url-to-fabric should NOT run)
- Response `result` field contains scraped content
- Standard fields present: `text`, `source_url`, `source_type`

### 5.2 Explicit Pipeline Execution (`/pipeline/{name}`)

**Purpose:** Execute a named pipeline orchestration.

**Test Case: URL-to-Fabric Pipeline**
```bash
curl -X POST http://localhost:8080/pipeline/url-to-fabric \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "payload": {
      "url": "https://mattjoyce.ai/",
      "pattern": "summarize"
    }
  }'
```

**Expected Result:**
- Multi-step execution: jina-reader → fabric
- Response `result` field contains final step output (fabric summary)
- Response `tree[]` array contains both steps
- Terminal step result is prioritized in top-level response
- Context fields propagated automatically (pattern visible in fabric step)

### 5.3 Legacy Trigger Endpoint (`/trigger/{plugin}/{command}`)

**Purpose:** Backward compatibility - may trigger pipelines if configured.

**Test Case: Trigger with Pipeline Listener**
```bash
curl -X POST http://localhost:8080/trigger/jina-reader/handle \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "payload": {
      "url": "https://example.com"
    }
  }'
```

**Expected Result:**
- Response header contains deprecation warning: `X-Ductile-Deprecation-Warning`
- If pipeline listens to `jina-reader.handle`, the pipeline executes
- Behavior matches historical routing semantics

---

## 6. Plugin Payload Spec Compliance

All plugins must adhere to the standard payload convention (see `PLUGIN_DEVELOPMENT.md`).

### 6.1 Standard Fields

Verify each plugin emits the following fields:

| Field | Type | Purpose |
|-------|------|---------|
| `text` | string | Primary content for downstream processing |
| `result` | string | Final human-readable output |
| `source_url` | string | Originating URL (if applicable) |
| `source_type` | string | Content origin: `web`, `youtube`, `file`, `llm` |

### 6.2 Event Type Naming

Event types must follow `<plugin_name>.<past_tense_verb>` convention:

| Plugin | Event Type |
|--------|------------|
| jina-reader | `jina_reader.scraped`, `jina_reader.changed` |
| youtube_transcript | `youtube_transcript.fetched` |
| fabric | `fabric.completed` |
| file_handler | `file_handler.read`, `file_handler.written` |

### 6.3 Validation Procedure

**Test:** Execute each plugin and inspect output event:

```bash
# Example: Verify jina-reader output
curl -X POST http://localhost:8080/plugin/jina-reader/handle \
  -H "Authorization: Bearer <token>" \
  -d '{"payload": {"url": "https://example.com"}}' | jq '.result'
```

**Check:**
- `jq '.result.text'` → Should contain primary content
- `jq '.result.source_url'` → Should match input URL
- `jq '.result.source_type'` → Should be `"web"`
- Inspect plugin code: No manual propagation loops (removed in PR #42)

---

## 7. Context Auto-Propagation Testing

The dispatcher automatically propagates these fields from input to output events:
- `pattern`
- `prompt`
- `model`
- `output_dir`
- `output_path`
- `filename`

### Test Procedure

**Setup:** Execute a pipeline with context fields in the initial payload.

```bash
curl -X POST http://localhost:8080/pipeline/url-to-fabric \
  -H "Authorization: Bearer <token>" \
  -d '{
    "payload": {
      "url": "https://mattjoyce.ai/",
      "pattern": "extract_wisdom",
      "model": "gpt-4"
    }
  }' | jq '.tree'
```

**Verify:**
- Second step (fabric) receives `pattern` and `model` fields
- No manual copying required in plugin code
- Inspect logs: `pattern` visible in fabric plugin execution

**Code Reference:** `internal/dispatch/dispatcher.go:648-661` (routeEvents function)

---

## 8. Terminal Step Result Validation

As of PR #42, API sync responses return the final step's result, not the root job.

### Test Case

**Execute multi-step pipeline:**
```bash
curl -X POST http://localhost:8080/pipeline/url-to-fabric \
  -H "Authorization: Bearer <token>" \
  -d '{
    "payload": {
      "url": "https://example.com",
      "pattern": "summarize"
    }
  }' > response.json
```

**Validate:**
```bash
# Top-level result should be fabric output (final step)
jq '.result.result' response.json  # Should contain summary text

# Tree should have both steps
jq '.tree | length' response.json  # Should be >= 2

# Root job is step 0 (jina-reader)
jq '.tree[0].plugin' response.json  # Should be "jina-reader"

# Terminal job is last step (fabric)
jq '.tree[-1].plugin' response.json  # Should be "fabric"
```

**Implementation:** `internal/api/handlers.go` (sync response assembly)

---

## 9. Plugin-Specific Tests

### 9.1 Jina-Reader
- **Input:** `{"url": "https://github.com/mattjoyce"}`
- **Output Fields:** `text`, `source_url`, `source_type: "web"`
- **Event:** `jina_reader.scraped` or `jina_reader.changed`

### 9.2 YouTube Transcript
- **Input:** `{"url": "https://youtube.com/watch?v=..."}`
- **Output Fields:** `text`, `transcript`, `source_url`, `source_type: "youtube"`
- **Event:** `youtube_transcript.fetched`

### 9.3 Fabric
- **Input:** `{"text": "...", "pattern": "summarize"}`
- **Output Fields:** `result`, `text` (alias), `source_type: "llm"`
- **Event:** `fabric.completed`

### 9.4 File Handler
- **Input Read:** `{"action": "read", "file_path": "test.md"}`
- **Output Fields:** `text`, `source_type: "file"`
- **Event:** `file_handler.read`

- **Input Write:** `{"action": "write", "file_path": "output.md", "content": "..."}`
- **Output Fields:** `result` (confirmation message)
- **Event:** `file_handler.written`

---

## 10. Integration Test Scenarios

### 10.1 URL Summarization Flow
**Pipeline:** url-to-fabric

```bash
curl -X POST http://localhost:8080/pipeline/url-to-fabric \
  -H "Authorization: Bearer <token>" \
  -d '{
    "payload": {
      "url": "https://mattjoyce.ai/",
      "pattern": "summarize"
    }
  }'
```

**Validate:**
1. Jina-reader scrapes URL
2. Fabric receives scraped text
3. Fabric uses `summarize` pattern
4. Final result contains summary
5. No manual field copying in plugin code

### 10.2 YouTube to Document
**Pipeline:** youtube-to-fabric (if configured)

```bash
curl -X POST http://localhost:8080/pipeline/youtube-to-fabric \
  -H "Authorization: Bearer <token>" \
  -d '{
    "payload": {
      "url": "https://youtube.com/watch?v=dQw4w9WgXcQ",
      "pattern": "extract_wisdom"
    }
  }'
```

**Validate:**
1. YouTube transcript fetched
2. Text propagated to fabric
3. Pattern applied
4. Terminal result returned

---

## 11. Regression Tests

### 11.1 Bug: Endpoint Ambiguity (Issue #102)
**Fixed in:** PR #43

**Test:** Verify `/plugin` endpoint does NOT trigger pipelines.

```bash
# This should run jina-reader ONLY
curl -X POST http://localhost:8080/plugin/jina-reader/handle \
  -H "Authorization: Bearer <token>" \
  -d '{"payload": {"url": "https://example.com"}}' | jq '.tree | length'
```

**Expected:** Tree length = 1 (only jina-reader)

### 11.2 Bug: Pipeline Trigger Assumption (Codex Finding #1)
**Fixed in:** Commit a78cc86

**Test:** Multi-entry pipeline resolves correctly.

**Code Inspection:** `internal/router/engine.go:GetEntryDispatches()`
- Should use `pipeline.EntryNodeIDs` to resolve entry points
- Should NOT assume trigger is "plugin.command" format and split it

### 11.3 Bug: Payload Wrapping Regression (Codex Finding #2)
**Fixed in:** Commit a78cc86

**Test:** `/pipeline` endpoint wraps `handle` commands correctly.

```bash
curl -X POST http://localhost:8080/pipeline/url-to-fabric \
  -H "Authorization: Bearer <token>" \
  -d '{"payload": {"url": "https://example.com"}}' -v 2>&1 | grep "HTTP/1.1 200"
```

**Expected:** 200 OK (not 400/500 from malformed payload)

### 11.4 Bug: Response Field Inconsistency (Codex Finding #3)
**Fixed in:** Commit 5e84254

**Test:** `/pipeline` response includes all job fields.

```bash
curl -X POST http://localhost:8080/pipeline/url-to-fabric \
  -H "Authorization: Bearer <token>" \
  -d '{"payload": {"url": "https://example.com"}}' | jq '.tree[0] | keys'
```

**Expected Keys:**
- `job_id`
- `plugin`
- `command`
- `status`
- `result`
- `last_error`
- `started_at`
- `completed_at`

---

## 12. Error Handling Tests

### 12.1 Invalid Plugin Name
```bash
curl -X POST http://localhost:8080/plugin/nonexistent/handle \
  -H "Authorization: Bearer <token>" \
  -d '{"payload": {}}'
```
**Expected:** 404 Not Found or meaningful error

### 12.2 Invalid Pipeline Name
```bash
curl -X POST http://localhost:8080/pipeline/does-not-exist \
  -H "Authorization: Bearer <token>" \
  -d '{"payload": {}}'
```
**Expected:** 404 Not Found or "pipeline not found" error

### 12.3 Missing Payload
```bash
curl -X POST http://localhost:8080/plugin/jina-reader/handle \
  -H "Authorization: Bearer <token>" \
  -d '{}'
```
**Expected:** Plugin executes but may return error if URL required

### 12.4 Malformed JSON
```bash
curl -X POST http://localhost:8080/plugin/jina-reader/handle \
  -H "Authorization: Bearer <token>" \
  -d 'not-json'
```
**Expected:** 400 Bad Request

---

## 13. Performance & Load Tests

### 13.1 Concurrent Plugin Calls
```bash
# Execute 10 concurrent direct plugin calls
for i in {1..10}; do
  curl -X POST http://localhost:8080/plugin/echo/health \
    -H "Authorization: Bearer <token>" &
done
wait
```

**Validate:** All requests succeed, no deadlocks

### 13.2 Pipeline Throughput
```bash
# Measure pipeline execution time
time curl -X POST http://localhost:8080/pipeline/url-to-fabric \
  -H "Authorization: Bearer <token>" \
  -d '{"payload": {"url": "https://example.com", "pattern": "summarize"}}'
```

**Baseline:** Establish baseline execution times for comparison after changes

---

## 14. Documentation Validation

### 14.1 Skill Manifest Accuracy
```bash
./ductile skill
```

**Verify:**
- Section 2 lists all plugins with `/plugin/{plugin}/{command}` format
- Plugins have correct `poll`, `handle`, `health` commands listed
- Descriptions are accurate

### 14.2 PLUGIN_DEVELOPMENT.md Compliance
**Check:** All example code in docs matches current implementation.

**Files to validate:**
- Event type naming examples
- Standard field examples
- Context propagation documentation
- No references to manual propagation loops

---

## 15. Test Checklist

Before releasing a new version, complete this checklist:

- [ ] `go test ./...` passes
- [ ] `go test ./cmd/ductile/...` passes
- [ ] Echo runbook completes successfully
- [ ] All three API endpoints tested (`/plugin`, `/pipeline`, `/trigger`)
- [ ] Plugin payload spec compliance verified (all 4 plugins)
- [ ] Context auto-propagation works in multi-step pipeline
- [ ] Terminal step result returned in sync response
- [ ] Deprecation header present on `/trigger` endpoint
- [ ] Regression tests pass (all Codex findings)
- [ ] Error handling behaves correctly
- [ ] `ductile skill` output is accurate
- [ ] Documentation matches implementation

---

## 16. CI/CD Pipeline Recommendations

### GitHub Actions Workflow
```yaml
name: Test Suite
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - run: go test ./...
      - run: go test ./cmd/ductile/...

  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - run: docker compose up -d
      - run: ./scripts/e2e-tests.sh
```

---

## 17. Troubleshooting

### Issue: Plugin not found
**Symptom:** `POST /plugin/my-plugin/handle` returns 404
**Check:**
- Plugin directory exists in `plugins/`
- Plugin has `manifest.yaml`
- `ductile config check` passes

### Issue: Pipeline doesn't trigger
**Symptom:** `POST /pipeline/my-pipeline` returns error
**Check:**
- Pipeline defined in `config/ductile/pipelines.yaml`
- Pipeline has valid entry points
- `ductile config check` shows no errors

### Issue: Context fields not propagating
**Symptom:** Second step in pipeline doesn't receive `pattern` field
**Debug:**
1. Inspect first step's output event
2. Check `internal/dispatch/dispatcher.go:648-661` is active
3. Verify field name matches designated list (pattern, prompt, model, output_dir, output_path, filename)

### Issue: Wrong result returned
**Symptom:** API returns intermediate step result instead of final
**Debug:**
1. Check `internal/router/interface.go` - pipeline has `TerminalStepIDs`?
2. Verify `internal/queue/queue.go` - joins with `event_context` table
3. Inspect API response `tree[]` array - which step is marked terminal?

---

## 18. Future Test Improvements

- Add automated E2E tests using Go test framework
- Create benchmark suite for performance regression detection
- Implement contract testing for plugin I/O schemas
- Add load testing with k6 or similar tool
- Create mutation testing for critical paths (dispatch, router)
- Implement property-based testing for event routing logic
