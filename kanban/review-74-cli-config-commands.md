---
id: 74
status: todo
priority: Normal
tags: [cli, config, review, ux, llm-operator]
---

# REVIEW: CLI Config Commands for LLM Operator Usability

Comprehensive review of senechal-gw CLI commands from an LLM operator perspective, focusing on validation, error handling, and usability.

## Executive Summary

**Overall Assessment: Good foundation, needs polish**

✅ **Strengths:**
- Clear NOUN VERB structure
- Machine-readable JSON output formats
- Good error messages with hints
- Proper exit codes
- Excellent lineage tracking (job inspect)

❌ **Issues Found:**
1. **CRITICAL BUG**: Schedule validation incorrectly warns "daily is very short (< 1m)"
2. `--json` flag doesn't work for `job inspect`
3. No `--help` support at verb level (`config check --help` fails)
4. `config set` command appears broken/unimplemented

## Detailed Testing Results

### 1. `config check` Command ✅

**Usage:**
```bash
senechal-gw config check [-config PATH] [-format human|json] [-strict]
```

**Positive findings:**
- ✅ Validates YAML syntax
- ✅ Detects missing required fields
- ✅ JSON output format works perfectly
- ✅ `-strict` flag treats warnings as errors (exit code 2)
- ✅ Good error messages: "schedule is required for enabled plugins"

**Test results:**
```bash
# Valid config
$ senechal-gw config check
Configuration valid (2 warning(s))
  WARN  [schedule] plugins.fabric.schedule.every: schedule interval "daily" is very short (< 1m)

# JSON format (perfect for LLM parsing)
$ senechal-gw config check -format json
{
  "valid": true,
  "warnings": [
    {
      "category": "schedule",
      "message": "schedule interval \"daily\" is very short (< 1m)",
      "field": "plugins.fabric.schedule.every"
    }
  ]
}

# Strict mode
$ senechal-gw config check -strict
Exit code: 2 (warnings treated as errors)

# Invalid config
$ senechal-gw config check --config /tmp/bad-config.yaml
Exit code: 1
Config load error: invalid configuration: plugin "file_handler": schedule is required for enabled plugins
```

**BUG FOUND:** 🐛
```
internal/doctor/doctor.go line ~XXX:
"schedule interval %q is very short (< 1m)"
```
This warning triggers for "daily" intervals, which is **completely wrong**. Daily = 24 hours, not < 1 minute. This is a duration parsing bug in the validation logic.

---

### 2. `config show` Command ✅

**Usage:**
```bash
senechal-gw config show [-config PATH]
```

**Positive findings:**
- ✅ Shows complete resolved configuration
- ✅ Includes all defaults and merged values
- ✅ Clean YAML output
- ✅ Useful for debugging config issues

**Test result:**
```yaml
service:
    name: senechal-gw
    tick_interval: 1m0s
    log_level: info
plugins:
    fabric:
        enabled: true
        schedule:
            every: daily
            jitter: 1h0s
        config:
            allowed_read_paths: /home/matt/admin/senechal-test/test-files
```

Perfect for LLM inspection of active configuration.

---

### 3. `config get` Command ✅

**Usage:**
```bash
senechal-gw config get <path.to.key>
```

**Positive findings:**
- ✅ Dot-notation path access
- ✅ Returns just the value (easy to parse)
- ✅ Useful for scripting and LLM queries

**Test result:**
```bash
$ senechal-gw config get plugins.fabric.enabled
true
```

**LLM Use Case:** Perfect for checking specific config values before operations.

---

### 4. `config lock` Command ✅

**Usage:**
```bash
senechal-gw config lock [-config PATH]
```

**Positive findings:**
- ✅ Updates integrity hashes
- ✅ Clear success message
- ✅ Authorizes current configuration state

**Test result:**
```bash
$ senechal-gw config lock
Successfully locked configuration in 1 directory/ies:
  - /home/matt/admin/senechal-test
```

**LLM Use Case:** After making config changes, lock the new state to prevent drift.

---

### 5. `config set` Command ❌

**Usage:**
```bash
senechal-gw config set <path.to.key> <value>
```

**Status:** **BROKEN/UNIMPLEMENTED**

**Test result:**
```bash
$ senechal-gw config set plugins.echo.enabled true
(No output, command appears to fail silently)
```

**Issue:** Command doesn't work. This is critical for LLM operators who need to programmatically update configuration.

**Expected behavior:**
- Set value in config file
- Validate the change
- Report success/failure
- Optionally require `config lock` after changes

---

### 6. `job inspect` Command ✅✅✅

**Usage:**
```bash
senechal-gw job inspect <job_id> [-config PATH] [-json]
```

**Positive findings:**
- ✅✅✅ **EXCELLENT for LLM operators**
- ✅ Shows complete pipeline lineage
- ✅ Displays baggage accumulation at each hop
- ✅ Shows parent-child relationships
- ✅ Includes workspace directories
- ✅ Pipeline name and step IDs visible
- ✅ Clear, structured output

**Test result:**
```
Lineage Report
Job ID      : da755120-72ac-473b-bb4b-37b6f74fb687
Plugin      : file_handler
Command     : handle
Status      : succeeded
Context ID  : 5711e73b-d243-426b-9720-c2ba2642b1ac
Hops        : 2

[1] file-to-report :: analyze
    context_id : 10d9c394-1be4-4e75-a0c9-ae0a09d7dd56
    parent_id  : <none>
    job        : d9b064f2-82de-4181-806d-e43b0b137a25 (fabric:handle, succeeded)
    workspace  : data/workspaces/d9b064f2-82de-4181-806d-e43b0b137a25
    baggage    :
      {
        "content": "...",
        "output_dir": "/home/matt/admin/senechal-test/reports",
        "pattern": "summarize"
      }

[2] file-to-report :: save
    context_id : 5711e73b-d243-426b-9720-c2ba2642b1ac
    parent_id  : 10d9c394-1be4-4e75-a0c9-ae0a09d7dd56
    job        : da755120-72ac-473b-bb4b-37b6f74fb687 (file_handler:handle, succeeded)
    baggage    :
      {
        "content": "...",
        "result": "ONE SENTENCE SUMMARY:\n...",
        "output_dir": "/home/matt/admin/senechal-test/reports"
      }
```

**BUG FOUND:** 🐛
The `--json` flag doesn't work - it just shows usage instead of JSON output.

**LLM Use Case:**
- Debugging pipeline failures
- Understanding data flow through multi-hop chains
- Verifying baggage propagation
- Inspecting workspace artifacts

---

## LLM Operator Usability Assessment

### ✅ What Works Well

1. **Structured Output:** JSON format makes parsing easy
2. **Exit Codes:** Proper use of exit codes for success/failure
3. **Error Messages:** Include hints and actionable information
4. **NOUN VERB Pattern:** Consistent command structure
5. **Validation:** Catches configuration errors before runtime
6. **Lineage Tracking:** Job inspect provides excellent debugging capability

### ❌ What Needs Improvement

1. **Help System:**
   - `config check --help` doesn't work (should show flags)
   - `config help` shows verbs but not flag details
   - Need per-verb help documentation

2. **Missing Features:**
   - No `--verbose` flag for detailed diagnostics
   - No `--dry-run` for validation without side effects
   - `config set` appears broken/unimplemented
   - `--json` flag doesn't work for `job inspect`

3. **Validation Bugs:**
   - Schedule interval validation incorrectly warns about "daily" being < 1m
   - No way to suppress specific warning categories

4. **Discovery:**
   - Hidden verbs (`show`, `get`, `set`) not listed in main help
   - "planned" commands (`plugin list`, `plugin run`) clutter help

### Recommendations for LLM-Friendly Improvements

1. **Add `--help` to all verbs:**
   ```bash
   senechal-gw config check --help
   # Should show: -config, -format, -strict
   ```

2. **Fix validation bug:**
   - Schedule "daily" should not trigger "< 1m" warning
   - Proper duration parsing needed

3. **Implement `config set` properly:**
   ```bash
   senechal-gw config set plugins.fabric.enabled false
   # Should: update config.yaml, validate, report success
   ```

4. **Fix `--json` flag for `job inspect`:**
   - Currently shows usage instead of JSON output
   - Should return structured JSON for programmatic parsing

5. **Add `--dry-run` where applicable:**
   - `config lock --dry-run` - preview what would be locked
   - `config set --dry-run` - preview changes without writing

6. **Improve help discoverability:**
   - List all verbs in `config help` with their flags
   - Remove "planned" commands from main help or mark clearly as unimplemented

## Priority Issues for Dev Team

| Priority | Issue | Impact |
|----------|-------|--------|
| **P0** | Schedule validation bug (daily < 1m) | Confusing false warnings |
| **P1** | `config set` broken/unimplemented | Cannot update config programmatically |
| **P1** | `--json` flag broken for `job inspect` | LLM parsing impossible |
| **P2** | No `--help` at verb level | Poor discoverability |
| **P3** | Hidden verbs not in main help | Discovery issue |

## Test Coverage

**Tested Commands:**
- ✅ `config check` (with -format, -strict, -config flags)
- ✅ `config show`
- ✅ `config get`
- ✅ `config lock`
- ❌ `config set` (broken)
- ✅ `job inspect` (human format working, JSON flag broken)

**Not Yet Tested:**
- `system status` (marked as "planned")
- `plugin list` (marked as "planned")
- `plugin run` (marked as "planned")

## Narrative

- 2026-02-12: Conducted comprehensive CLI review from LLM operator perspective. Tested config commands (check, show, get, lock, set) and job inspect. Found critical schedule validation bug where "daily" incorrectly warns as "< 1m". Discovered config set is broken/unimplemented and --json flag doesn't work for job inspect. Overall assessment: good foundation with clear NOUN VERB structure and proper exit codes, but needs polish for production LLM operator use. The job inspect command is particularly excellent for debugging pipeline lineage and baggage flow. Created detailed recommendations for improving help system, validation, and programmatic access. (by @test-admin)
