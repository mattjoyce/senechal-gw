---
id: 73
status: todo
priority: Normal
tags: [bug, plugin, file_handler, fabric, pipeline]
---

# BUG: file_handler write prefers 'content' over 'result', writes wrong data

## Description

When file_handler receives both `content` and `result` fields (via Smart Core baggage merge), it incorrectly prefers `content` (original input) over `result` (fabric analysis output), causing the wrong data to be written to the report file.

## Impact

- **Severity**: Normal - Specific to file_handler→fabric→file_handler pipeline
- **Scope**: Any pipeline where fabric processes text and file_handler writes the result
- **User Experience**: Report files contain original input instead of AI analysis

## Evidence

Full E2E test with Smart Core merge working:

1. file_handler reads input (484 bytes)
2. Fabric analyzes and produces result (1059 bytes)
3. Smart Core merges both fields into final step payload:
   - `content`: 484 bytes (original text from step 1)
   - `result`: 1059 bytes (fabric analysis from step 2)
4. file_handler write uses: `content = payload.get("content") or payload.get("result")`
5. Since `content` exists, it's used (wrong!)
6. File written with 484 bytes of original input, not 1059 bytes of analysis

**Database evidence:**
```sql
SELECT length(json_extract(accumulated_json, '$.content')),
       length(json_extract(accumulated_json, '$.result'))
FROM event_context WHERE pipeline_name = 'file-to-report' AND step_id = 'save';
-- Result: 484|1059
```

**Actual file written:**
```bash
$ wc -c ~/admin/senechal-test/reports/summarize_20260212_032338.md
484  # Contains original input, not fabric analysis!
```

## Root Cause

**File:** `plugins/file_handler/run.py`, line 144

```python
# Current (wrong):
content = payload.get("content") or payload.get("result")

# Should be:
content = payload.get("result") or payload.get("content")
```

The `or` operator returns the first truthy value. Since both fields exist after Smart Core merge, `content` wins. But for fabric analysis pipelines, we want `result` to take priority.

## Expected Behavior

When both `result` and `content` are present, prefer `result` (processed/analyzed data) over `content` (raw input data).

## Reproduction

```bash
cd ~/admin/senechal-test
./senechal-gw system start --config config.yaml &

curl -X POST http://localhost:8080/trigger/file_handler/handle \
  -H "Authorization: Bearer test_admin_token_local" \
  -d '{
    "payload": {
      "action": "read",
      "file_path": "/home/matt/admin/senechal-test/test-files/sample.md",
      "pattern": "summarize",
      "output_dir": "/home/matt/admin/senechal-test/reports"
    }
  }'

# Wait for pipeline to complete
sleep 45

# Check file - will contain original 484B input, not 1059B fabric analysis
cat ~/admin/senechal-test/reports/summarize_*.md
```

## Suggested Fix

Change line 144 in `plugins/file_handler/run.py`:

```python
# Prefer 'result' (processed data) over 'content' (raw input)
content = payload.get("result") or payload.get("content")
```

This way:
- If fabric emitted `result`, use it (analysis output)
- Otherwise fall back to `content` (for non-fabric writes)

## Related

- Card #72: Smart Core baggage merge (enabled this bug to be discovered)
- Card #62: Multi-Plugin Pipeline E2E Test (where this was discovered)
- `internal/dispatch/dispatcher.go`: Smart Core merge implementation

## Narrative

- 2026-02-12: Discovered during verification testing of bug #72 fix. After Smart Core merge was implemented, full 3-hop pipeline executed successfully with proper field propagation. However, output file contained original input (484B) instead of fabric analysis (1059B). Investigation showed both fields present in final payload, but wrong field priority in plugin code. The Smart Core merge is working correctly - this is a plugin-level logic bug. (by @test-admin)
