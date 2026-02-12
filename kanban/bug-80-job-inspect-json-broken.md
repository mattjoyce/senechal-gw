---
id: 80
status: done
priority: High
tags: [bug, cli, json, llm-operator, observability]
---

# BUG: job inspect --json flag doesn't work

## Description

The `job inspect` command's `--json` flag doesn't produce JSON output. This was originally reported in card #74 and marked as implemented in card #61, but testing shows it still doesn't work.

## Impact

- **Severity**: High - Blocks LLM automation
- **Scope**: Job inspection and lineage analysis
- **User Experience**: LLM operators cannot parse job data programmatically

## Evidence

**From card #74 (2026-02-12):**
> **BUG FOUND:** 🐛
> The `--json` flag doesn't work - it just shows usage instead of JSON output.

**Current testing (2026-02-12):**
```bash
# Create a job
$ curl -X POST http://localhost:8080/trigger/echo/poll \
  -H "Authorization: Bearer test_admin_token_local" \
  -d '{"message": "test"}'
{"job_id":"86beb9ef-a18f-4032-9a3f-e08be3e76783","status":"queued"}

# Try to inspect with --json
$ ./senechal-gw job inspect 86beb9ef-a18f-4032-9a3f-e08be3e76783 --json
Inspect failed: job "86beb9ef-a18f-4032-9a3f-e08be3e76783" has no event_context_id

# Note: Could not fully test due to jobs lacking context IDs
# But card #74 reported it showed usage errors instead of JSON
```

## Expected Behavior

**From card #61 specification:**
- `senechal-gw job inspect <id> --json` returns structured JSON
- JSON output includes baggage, artifacts, and job metadata for each hop
- Follows CLI design principle: all "Read" actions must support `--json`

**Example expected output:**
```json
{
  "job_id": "da755120-72ac-473b-bb4b-37b6f74fb687",
  "plugin": "file_handler",
  "command": "handle",
  "status": "succeeded",
  "context_id": "5711e73b-d243-426b-9720-c2ba2642b1ac",
  "hops": 2,
  "lineage": [
    {
      "step": 1,
      "pipeline": "file-to-report",
      "stage": "analyze",
      "context_id": "10d9c394-1be4-4e75-a0c9-ae0a09d7dd56",
      "parent_id": null,
      "job_id": "d9b064f2-82de-4181-806d-e43b0b137a25",
      "plugin": "fabric",
      "command": "handle",
      "status": "succeeded",
      "workspace": "data/workspaces/d9b064f2-82de-4181-806d-e43b0b137a25",
      "baggage": {
        "content": "...",
        "output_dir": "/home/matt/admin/senechal-test/reports",
        "pattern": "summarize"
      }
    },
    {
      "step": 2,
      "pipeline": "file-to-report",
      "stage": "save",
      "context_id": "5711e73b-d243-426b-9720-c2ba2642b1ac",
      "parent_id": "10d9c394-1be4-4e75-a0c9-ae0a09d7dd56",
      "job_id": "da755120-72ac-473b-bb4b-37b6f74fb687",
      "plugin": "file_handler",
      "command": "handle",
      "status": "succeeded",
      "baggage": {
        "content": "...",
        "result": "ONE SENTENCE SUMMARY:\n...",
        "output_dir": "/home/matt/admin/senechal-test/reports"
      }
    }
  ]
}
```

## Status from Related Cards

- **Card #61** (id: 61, status: done): "Implement mandatory --json for job inspect"
  - Marked as DONE by @gemini
  - But testing shows it doesn't work

- **Card #74** (id: 74, status: done): CLI review by @test-admin
  - Found: "--json flag doesn't work for job inspect"
  - Reported it showed usage instead of JSON output

## Root Cause (Suspected)

**Likely issue:** Flag parsing problem in `cmd/senechal-gw/job.go`:

```go
// Probably missing or broken:
func runJobInspect(jobID string, jsonOutput bool) error {
    lineage, err := getJobLineage(jobID)

    if jsonOutput {
        // This might be missing or broken
        outputJSON(lineage)
        return nil
    }

    // Human-readable output (this works)
    printLineage(lineage)
    return nil
}
```

**Or flag registration issue:**
```go
// Flag might not be registered correctly
inspectCmd.Flags().BoolP("json", "j", false, "JSON output")
// Missing: inspectCmd.MarkFlagRequired("json")? Or not parsing?
```

## LLM Operator Use Case

**Why this matters:**

LLM operators need programmatic access to job lineage for:
1. Debugging pipeline failures
2. Understanding data flow through multi-hop chains
3. Verifying baggage propagation
4. Inspecting workspace artifacts
5. Building automated monitoring

**Current workaround:** None - must parse human-readable output (brittle)

## Testing Limitations

**Note:** During comprehensive CLI testing, I couldn't fully verify this bug because:
- Jobs created via `/trigger` endpoint don't have `event_context_id`
- `job inspect` requires context ID (set by pipeline routing)
- Single-step jobs don't get context IDs

**To properly test:**
1. Create a multi-hop pipeline with routing
2. Trigger the pipeline
3. Wait for completion
4. Inspect the final job with `--json`

## Reproduction (When Pipeline Available)

```bash
# Setup: Create multi-hop pipeline config
# (Requires routing setup)

# Trigger pipeline job
curl -X POST http://localhost:8080/trigger/fabric/handle \
  -H "Authorization: Bearer test_admin_token_local" \
  -d '{"pattern": "summarize", "content": "test"}'

# Wait for completion
sleep 5

# Get job ID from events or logs
JOB_ID="<job-id-from-response>"

# Test human-readable (should work)
./senechal-gw job inspect $JOB_ID

# Test JSON output (broken)
./senechal-gw job inspect $JOB_ID --json
# Expected: Valid JSON
# Actual: Usage message or error
```

## Testing Recommendations

After fix, verify:
1. ✅ `job inspect <id>` shows human-readable output (baseline)
2. ✅ `job inspect <id> --json` shows valid JSON
3. ✅ JSON output includes all fields (baggage, artifacts, metadata)
4. ✅ JSON is parseable by `jq` or `json.loads()`
5. ✅ Works with multi-hop pipeline jobs
6. ✅ Works with single-step jobs (when context available)
7. ✅ Exit code 0 on success, 1 on job not found

## Narrative

- 2026-02-12: Re-discovered during comprehensive CLI testing. This bug was originally reported in card #74 (CLI review) but marked as implemented in card #61 (by @gemini). However, testing shows the --json flag still doesn't produce JSON output. Could not fully verify the exact failure mode during testing because jobs created via /trigger endpoint lack event_context_id (required for lineage inspection). Card #74 reported it showed "usage message" instead of JSON. This blocks LLM operator automation for job lineage analysis, which is a critical observability feature. The human-readable output works excellently (as noted in card #74), but programmatic access is broken. (by @test-admin)
- 2026-02-13: Fixed fallback behavior for jobs that do not have `event_context_id` (common for direct trigger jobs). `job inspect --json` now returns valid structured JSON with `hops: 0` and empty `steps` instead of failing hard. Added regression coverage in inspect report tests. (by @assistant)
