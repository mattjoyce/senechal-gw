---
id: 62
status: doing
priority: Normal
blocked_by: []
assignee: "@test-admin"
tags: [pipeline, plugin, testing, e2e, wip]
---

# Multi-Plugin Pipeline E2E Test

Build and test a real multi-plugin pipeline: manual trigger → read file → fabric analyse → write report.

## Job Story

When I want to validate that pipelines work end-to-end with real plugins, I want a concrete pipeline that reads a local file, processes it through fabric, and saves the output, so I can prove the routing and workspace system works in practice.

## Pipeline Flow

```
POST /trigger/file_handler/read  (with file_path + pattern in payload)
  → file_handler(read)           reads file, emits file.read event
  → fabric(execute)              analyses text with pattern, emits fabric.completed
  → file_handler(write)          writes report to output folder, emits file.written
```

## Deliverables

### 1. `file_handler` plugin

**Language:** Python (match fabric plugin style)

**Manifest** (`plugins/file_handler/manifest.yaml`):
```yaml
name: file_handler
version: 0.1.0
protocol: 1
entrypoint: run.py
description: "Read and write local files with path restrictions"
commands:
  - name: read
    type: read
  - name: write
    type: write
  - name: health
    type: read
config_keys:
  required: []
  optional: [allowed_read_paths, allowed_write_paths]
```

**`read` command:**
- Input event payload: `{"file_path": "/abs/path/to/file.txt"}`
- Validates `file_path` resolves (via `os.path.realpath`) under one of `config.allowed_read_paths`
- Reads file contents as UTF-8 text
- Emits event:
  ```json
  {
    "type": "file.read",
    "payload": {
      "file_path": "/abs/path/to/file.txt",
      "filename": "file.txt",
      "content": "<file contents>",
      "size_bytes": 1234
    }
  }
  ```
- State updates: `last_read`, `reads_count`
- Error (non-retryable) if path not allowed or file not found

**`write` command:**
- Input event payload: `{"content": "...", "output_path": "/abs/path/report.md"}` (or `output_dir` + auto-generated filename)
- Validates `output_path` resolves under one of `config.allowed_write_paths`
- Creates parent directories if needed
- Writes content as UTF-8 text
- Emits event:
  ```json
  {
    "type": "file.written",
    "payload": {
      "file_path": "/abs/path/report.md",
      "size_bytes": 5678
    }
  }
  ```
- State updates: `last_write`, `writes_count`
- Error (non-retryable) if path not allowed

**`health` command:**
- Validates configured paths exist and are accessible

**Security:**
- All paths resolved to absolute via `os.path.realpath()` before prefix check (prevents symlink escape)
- Reject any path not under an allowed prefix
- No shell expansion, no glob in file_handler itself

### 2. Pipeline definition

Create `pipelines/file-to-report.yaml`:
```yaml
pipelines:
  - name: file-to-report
    on: file.read
    steps:
      - id: analyse
        uses: fabric
      - id: save
        uses: file_handler
```

**Event flow mapping:**
- Trigger `file_handler/read` via API → emits `file.read`
- Pipeline `file-to-report` triggers on `file.read`
- Step `analyse`: dispatcher calls `fabric` with `execute` command. The `file.read` payload must include `text` (mapped from `content`) and `pattern` fields. **Decision needed:** either:
  - (a) Include `pattern` in the original trigger payload and propagate via baggage, or
  - (b) Hardcode a default pattern in fabric config, or
  - (c) Have `file_handler` read forward the `pattern` field from its own event payload into the emitted event
- Option (c) is simplest: the trigger payload includes `{"file_path": "...", "pattern": "summarize"}`, file_handler passes `pattern` through in the `file.read` event payload
- Step `save`: dispatcher calls `file_handler` with `write` command. The `fabric.completed` payload has `result` (the report text). **The write step needs an output path** — propagate via baggage from the original trigger, or use a default from config

### 3. Config changes

Update `config.yaml` to register file_handler:
```yaml
plugins:
  file_handler:
    enabled: true
    timeout: 30s
    max_attempts: 1
    config:
      allowed_read_paths: "/Volumes/Projects/notes,/Volumes/Projects/papers"
      allowed_write_paths: "/Volumes/Projects/reports"

  fabric:
    enabled: true
    timeout: 120s
    max_attempts: 1
    config:
      # FABRIC_DEFAULT_MODEL: optional
```

Enable API:
```yaml
api:
  enabled: true
  listen: "localhost:8080"
  auth:
    tokens:
      - token: ${SENECHAL_TOKEN_ADMIN}
        scopes: ["*"]
```

### 4. Event payload mapping

The pipeline DSL uses `uses:` which dispatches to the plugin's default write command. The mapping between event payloads across hops needs care:

| Hop | Plugin Command | Receives in `event.payload` | Emits event type | Emits in payload |
|-----|---------------|---------------------------|-----------------|-----------------|
| 1 | `file_handler` `read` | `file_path`, `pattern`, `output_dir` | `file.read` | `content`, `pattern`, `output_dir`, `filename`, `size_bytes` |
| 2 | `fabric` `execute` | `text`*, `pattern`, `model` | `fabric.completed` | `result`, `pattern`, `model`, `input_length`, `output_length` |
| 3 | `file_handler` `write` | `content`*, `output_path` or `output_dir` | `file.written` | `file_path`, `size_bytes` |

**Key mapping issues:**
- `file_handler` emits `content` but fabric expects `text` → file_handler should emit both `content` and `text` (same value) for compatibility, OR fabric should also accept `content` as a fallback
- `fabric` emits `result` but file_handler write expects `content` → file_handler write should accept `result` as fallback for `content`
- `output_dir`/`output_path` must propagate through baggage (event_context) from the original trigger to the final write step

### 5. Install obsave plugin

- Source at `/mnt/Projects/obsave/` (path not currently accessible — needs mount or correct path)
- Once accessible: copy/symlink into `plugins/` directory, verify manifest, add config entry
- Could replace or extend the `file_handler(write)` step if obsave handles file output

### 6. Manual test procedure

```bash
# 1. Start the service with API enabled
export SENECHAL_TOKEN_ADMIN="test-token-123"
./senechal-gw start --config config.yaml

# 2. Trigger the pipeline
curl -X POST http://localhost:8080/trigger/file_handler/read \
  -H "Authorization: Bearer test-token-123" \
  -H "Content-Type: application/json" \
  -d '{
    "payload": {
      "file_path": "/Volumes/Projects/notes/some-note.md",
      "pattern": "summarize",
      "output_dir": "/Volumes/Projects/reports"
    }
  }'

# 3. Check job status (use returned job_id)
curl http://localhost:8080/job/<job_id> \
  -H "Authorization: Bearer test-token-123"

# 4. Inspect full pipeline lineage
./senechal-gw inspect <job_id>

# 5. Verify output
ls /Volumes/Projects/reports/
cat /Volumes/Projects/reports/<generated-report>.md
```

### 7. Automated test (optional)

Extend `internal/e2e/pipeline_test.go` with a test that uses temp directories for allowed paths, a mock fabric script, and verifies the full 3-hop chain writes output.

## Open Questions

- [ ] Fabric pattern to use for testing (`summarize`, `extract_wisdom`, `analyze_paper`?)
- [ ] Payload field mapping: should file_handler emit `text` to match fabric, or should fabric accept `content` as fallback?
- [ ] Output filename generation: timestamp-based? derive from input filename?
- [ ] obsave plugin: confirm correct source path
- [ ] Max file size limit for file_handler read? (prevent accidentally reading huge files)

## Narrative

- 2026-02-12: Started implementing card #62. Created Pythonic file_handler plugin with comprehensive error handling, type hints, security validation (realpath-based path checking). Set up Docker test environment. Discovered and fixed multiple issues: fabric manifest format (#63), fabric command mismatch (#66), API trigger failure (#68). After rebasing to get Sprint 4 routing implementation, discovered routing requires Pipeline DSL files in `pipelines/` directory, not the legacy `routes:` config. Created `pipelines/file-to-report.yaml` with pipeline definition. Updated Dockerfile to copy pipelines directory to container. Successfully validated E2E routing: file_handler → fabric routing works correctly. Router matched `file.read` event, created downstream job with proper traceability (event_context_id, pipeline/step metadata). Fabric job failed due to missing fabric binary (expected, external tool), but routing infrastructure validated successfully. Next: Either install fabric binary in container for full 3-hop test, or create mock fabric for testing purposes. (by @test-admin)
