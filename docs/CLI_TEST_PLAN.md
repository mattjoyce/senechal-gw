# CLI Test Plan

**Purpose:** Comprehensive, repeatable testing checklist for senechal-gw CLI commands.

**Audience:** Test lead (@test-admin) for efficient regression testing.

**Last Updated:** 2026-02-12

---

## Quick Test Environment Setup

```bash
# Navigate to test environment
cd ~/admin/senechal-test

# Verify binary exists
ls -lh senechal-gw

# Check config is valid baseline
./senechal-gw config check

# Start gateway if needed (for job testing)
./senechal-gw system start &
GW_PID=$!
sleep 2

# Cleanup function
cleanup() {
    kill $GW_PID 2>/dev/null
}
trap cleanup EXIT
```

---

## Test Matrix

| Command | Flags | Exit Codes | JSON Output | Backup | Validation |
|---------|-------|------------|-------------|--------|------------|
| config check | --config, --format, --strict | 0,1,2 | ✅ | N/A | N/A |
| config show | --config | 0,1 | ❌ | N/A | N/A |
| config get | --config | 0,1 | ❌ | N/A | N/A |
| config set | --config, --dry-run, --apply | 0,1 | ❌ | 🐛 #78 | 🐛 #79 |
| config lock | --config, -v | 0,1 | ❌ | N/A | N/A |
| job inspect | --config, --json | 0,1 | 🐛 #80 | N/A | N/A |
| system start | --config, --log-level | 0,1 | N/A | N/A | N/A |

**Legend:** ✅ Works | ❌ Not implemented | 🐛 Bug

---

## Test Suite 1: config check

**Purpose:** Validate configuration files

### Test 1.1: Basic validation (valid config)
```bash
./senechal-gw config check
# Expected: "Configuration valid."
# Exit code: 0
```

### Test 1.2: JSON output format
```bash
./senechal-gw config check --format json
# Expected: Valid JSON with "valid": true
# Exit code: 0
# Parse: echo $output | jq -e '.valid == true'
```

### Test 1.3: Strict mode (no warnings)
```bash
./senechal-gw config check --strict
# Expected: "Configuration valid."
# Exit code: 0
```

### Test 1.4: Invalid config detection
```bash
# Create invalid config
cp config.yaml config.yaml.backup
echo "invalid: [unclosed" >> config.yaml

./senechal-gw config check
# Expected: Error message about YAML syntax
# Exit code: 1

# Restore
mv config.yaml.backup config.yaml
```

### Test 1.5: Missing required fields
```bash
# Create config without required schedule
cat > /tmp/test-config.yaml <<EOF
plugins:
  echo:
    enabled: true
    # Missing schedule!
EOF

./senechal-gw config check --config /tmp/test-config.yaml
# Expected: "schedule is required for enabled plugins"
# Exit code: 1
```

### Test 1.6: Regression - Schedule validation (bug from card #74)
```bash
# Verify "daily" doesn't trigger false warning
grep -A3 "schedule:" config.yaml | grep "daily"

./senechal-gw config check
# Expected: No warning about "daily is very short (< 1m)"
# If warning appears: REGRESSION of fixed bug
```

**Pass Criteria:** All 6 tests pass

---

## Test Suite 2: config show

**Purpose:** Display resolved configuration

### Test 2.1: Show full config
```bash
./senechal-gw config show > /tmp/config-output.yaml
# Expected: Valid YAML output
# Verify: cat /tmp/config-output.yaml | grep "service:"
```

### Test 2.2: Config includes defaults
```bash
./senechal-gw config show | grep "tick_interval"
# Expected: "tick_interval: 1m0s" (default value)
```

### Test 2.3: Config shows merged values
```bash
./senechal-gw config show | grep "log_level"
# Expected: log_level value from config or default
```

**Pass Criteria:** All 3 tests pass

---

## Test Suite 3: config get

**Purpose:** Read specific config values

### Test 3.1: Get boolean value
```bash
result=$(./senechal-gw config get plugins.fabric.enabled)
# Expected: "true" or "false"
# Verify: [[ "$result" == "true" ]] || [[ "$result" == "false" ]]
```

### Test 3.2: Get string value
```bash
result=$(./senechal-gw config get service.log_level)
# Expected: "info", "debug", etc.
echo $result | grep -E "^(debug|info|warn|error)$"
```

### Test 3.3: Get duration value
```bash
result=$(./senechal-gw config get service.tick_interval)
# Expected: Duration format like "1m0s"
echo $result | grep -E "[0-9]+(ms|s|m|h)"
```

### Test 3.4: Nonexistent key (error handling)
```bash
./senechal-gw config get nonexistent.key.path 2>&1
# Expected: Error message with "not found"
# Exit code: 1
```

### Test 3.5: Nested path access
```bash
result=$(./senechal-gw config get plugins.fabric.schedule.every)
# Expected: Schedule value like "daily" or "1h"
[[ -n "$result" ]]  # Not empty
```

**Pass Criteria:** All 5 tests pass

---

## Test Suite 4: config set

**Purpose:** Modify configuration values

**⚠️ WARNING:** This command has known bugs (#78, #79). Test carefully.

### Test 4.1: Dry-run mode (no modification)
```bash
# Capture current value
before=$(./senechal-gw config get plugins.echo.enabled)

# Try dry-run
./senechal-gw config set --dry-run plugins.echo.enabled=true
# Expected: "Dry-run: would set..." message
# Expected: "Configuration check PASSED"

# Verify no change
after=$(./senechal-gw config get plugins.echo.enabled)
[[ "$before" == "$after" ]]
```

### Test 4.2: Apply changes (valid value)
```bash
# Set a safe value
./senechal-gw config set --apply service.log_level=debug
# Expected: "Successfully set..."
# Exit code: 0

# Verify change persisted
result=$(./senechal-gw config get service.log_level)
[[ "$result" == "debug" ]]

# Restore original
./senechal-gw config set --apply service.log_level=info
```

### Test 4.3: 🐛 Bug #78 - Backup creation
```bash
# Remove any existing backups
rm -f config.yaml.bak

# Modify config
./senechal-gw config set --apply service.log_level=debug

# Check for backup
ls config.yaml.bak
# Expected: Backup file exists
# KNOWN BUG: Backup NOT created (bug #78)
# Status: 🐛 FAIL (expected until fixed)
```

### Test 4.4: 🐛 Bug #79 - Validation before write
```bash
# Try to set invalid value (enable plugin without schedule)
./senechal-gw config set --apply plugins.echo.enabled=true
# Expected: Validation error BEFORE writing
# KNOWN BUG: Writes anyway, corrupts config (bug #79)
# Status: 🐛 FAIL (expected until fixed)

# If bug present, manually fix
sed -i 's/enabled: true/enabled: false/' config.yaml
./senechal-gw config check  # Verify fixed
```

### Test 4.5: Flag position requirement
```bash
# Wrong: flag after path=value
./senechal-gw config set plugins.echo.enabled=true --apply 2>&1
# Expected: Usage error
# Exit code: 1

# Correct: flag before path=value
./senechal-gw config set --apply plugins.echo.enabled=false
# Expected: Success (if value valid)
```

### Test 4.6: Requires --dry-run or --apply flag
```bash
./senechal-gw config set plugins.echo.enabled=false 2>&1
# Expected: Error "either --dry-run or --apply must be specified"
# Exit code: 1
```

**Pass Criteria:** 4/6 tests pass (tests 4.3, 4.4 expected to fail due to known bugs)

---

## Test Suite 5: config lock

**Purpose:** Update configuration integrity hashes

### Test 5.1: Basic lock operation
```bash
./senechal-gw config lock
# Expected: "Successfully locked configuration in 1 directory/ies"
# Exit code: 0
```

### Test 5.2: Verbose mode
```bash
./senechal-gw config lock -v 2>&1 | tee /tmp/lock-output.txt
# Expected: "Processing directory:" message
# Expected: "WROTE .checksums:" message
grep "WROTE .checksums" /tmp/lock-output.txt
```

### Test 5.3: Checksums file created
```bash
./senechal-gw config lock

# Verify file exists
ls -la .checksums

# Verify format
cat .checksums
# Expected: YAML with "version:", "generated_at:", "hashes:"
grep "version: 1" .checksums
```

### Test 5.4: Regression - Hardcoded filenames (bug #76, cancelled)
```bash
./senechal-gw config lock -v 2>&1 | grep "tokens.yaml"
# Expected: "SKIP tokens.yaml: not found (optional)"
# Note: This is bug #76 but user cancelled it, so this is expected behavior
```

**Pass Criteria:** All 4 tests pass (test 5.4 is regression check only)

---

## Test Suite 6: job inspect

**Purpose:** Display job lineage and execution details

**⚠️ PREREQUISITE:** Gateway must be running and jobs must have been executed via pipelines (with context IDs).

### Test 6.1: Create test job
```bash
# Ensure gateway running
curl -s http://localhost:8080/health || ./senechal-gw system start &

# Trigger job
JOB_ID=$(curl -s -X POST http://localhost:8080/trigger/fabric/handle \
  -H "Authorization: Bearer test_admin_token_local" \
  -H "Content-Type: application/json" \
  -d '{"pattern": "test", "content": "test content"}' | jq -r '.job_id')

echo "Job ID: $JOB_ID"

# Wait for job completion
sleep 5
```

### Test 6.2: Human-readable output
```bash
./senechal-gw job inspect $JOB_ID
# Expected: Lineage report with:
#   - Job ID, Plugin, Command, Status
#   - Context ID, Hops
#   - Pipeline stages with baggage
# Note: May fail with "no event_context_id" if job not routed via pipeline
```

### Test 6.3: 🐛 Bug #80 - JSON output
```bash
./senechal-gw job inspect $JOB_ID --json
# Expected: Valid JSON output
# KNOWN BUG: Doesn't produce JSON (bug #80)
# Status: 🐛 FAIL (expected until fixed)

# If working, verify JSON:
./senechal-gw job inspect $JOB_ID --json | jq -e '.job_id'
```

### Test 6.4: Nonexistent job ID (error handling)
```bash
./senechal-gw job inspect 00000000-0000-0000-0000-000000000000 2>&1
# Expected: Error message about job not found
# Exit code: 1
```

**Pass Criteria:** 2/4 tests pass (test 6.3 expected to fail due to known bug, test 6.2 may fail if no pipeline routing)

**Note:** If tests 6.2/6.3 fail with "no event_context_id", this indicates jobs aren't being routed through pipelines. This is a test environment limitation, not a bug.

---

## Test Suite 7: system start

**Purpose:** Start gateway service

### Test 7.1: Gateway starts successfully
```bash
# Kill any existing instance
pkill -f senechal-gw

# Start in background
./senechal-gw system start > /tmp/gateway.log 2>&1 &
GW_PID=$!
sleep 2

# Verify process running
ps -p $GW_PID
# Expected: Process exists
```

### Test 7.2: API endpoints respond
```bash
# Test trigger endpoint
curl -s -X POST http://localhost:8080/trigger/echo/poll \
  -H "Authorization: Bearer test_admin_token_local" \
  -d '{"message": "test"}' | jq -e '.job_id'

# Expected: Returns job_id in JSON
# Exit code: 0
```

### Test 7.3: Authentication works
```bash
# Without auth header
curl -s -X POST http://localhost:8080/trigger/echo/poll \
  -d '{"message": "test"}' | grep "missing Authorization header"

# Expected: Error about missing auth
```

### Test 7.4: Gateway logs to file
```bash
# Start with explicit log file
pkill -f senechal-gw
./senechal-gw system start > gateway-test.log 2>&1 &
sleep 2

# Verify log file created and has content
[[ -f gateway-test.log ]] && [[ -s gateway-test.log ]]
```

**Pass Criteria:** All 4 tests pass

---

## Test Suite 8: Help System

**Purpose:** Verify discoverability and help text

### Test 8.1: Main help
```bash
./senechal-gw --help
# Expected: Lists all nouns (system, config, job, plugin)
grep -E "(system|config|job|plugin)" <(./senechal-gw --help)
```

### Test 8.2: Noun-level help
```bash
./senechal-gw config help
# Expected: Lists all config actions
grep -E "(lock|check|show|get|set)" <(./senechal-gw config help)
```

### Test 8.3: Improvement #81 - Action-level help
```bash
./senechal-gw config check --help 2>&1
# Expected: Shows flags for config check
# Expected: Contains "Usage: senechal-gw config check"
# Status: ✅ PASS
```

### Test 8.4: Version info
```bash
./senechal-gw --version
# Expected: Contains version, commit, and built_at lines
grep -E "(^senechal-gw |^commit:|^built_at:)" <(./senechal-gw --version)

./senechal-gw version --json
# Expected: Machine-readable version metadata keys
grep -E "\"version\"|\"commit\"|\"build_time\"" <(./senechal-gw version --json)
```

**Pass Criteria:** All 4 tests pass

---

## Regression Test Checklist

**Run before each release:**

1. [ ] All Test Suite 1 tests pass (config check)
2. [ ] All Test Suite 2 tests pass (config show)
3. [ ] All Test Suite 3 tests pass (config get)
4. [ ] Test Suite 4: 4/6 pass (bugs #78, #79 tracked)
5. [ ] All Test Suite 5 tests pass (config lock)
6. [ ] Test Suite 6: 2/4 pass (bug #80 tracked, context ID limitation documented)
7. [ ] All Test Suite 7 tests pass (system start)
8. [ ] All Test Suite 8 tests pass

**Known failures to track:**
- Test 4.3: Bug #78 (config set no backup)
- Test 4.4: Bug #79 (config set no validation)
- Test 6.3: Bug #80 (job inspect --json)

**When bugs are fixed:**
- Update test expectations from "🐛 FAIL" to "✅ PASS"
- Update test matrix at top of document
- Re-run full regression suite

---

## Test Execution Script

```bash
#!/bin/bash
# run-cli-tests.sh - Automated CLI test execution

set -e

PASS=0
FAIL=0
SKIP=0

test_result() {
    local name=$1
    local result=$2
    local expected_fail=${3:-false}

    if [[ "$result" == "pass" ]]; then
        if [[ "$expected_fail" == "true" ]]; then
            echo "  ⚠️  $name: PASSED (expected to fail - bug fixed?)"
        else
            echo "  ✅ $name: PASS"
        fi
        ((PASS++))
    else
        if [[ "$expected_fail" == "true" ]]; then
            echo "  🐛 $name: FAIL (expected - known bug)"
            ((SKIP++))
        else
            echo "  ❌ $name: FAIL"
            ((FAIL++))
        fi
    fi
}

echo "=== CLI Test Suite ==="
echo

# Test Suite 1: config check
echo "Suite 1: config check"
./senechal-gw config check >/dev/null 2>&1 && test_result "1.1 Basic validation" "pass" || test_result "1.1 Basic validation" "fail"
./senechal-gw config check --format json | jq -e '.valid' >/dev/null 2>&1 && test_result "1.2 JSON format" "pass" || test_result "1.2 JSON format" "fail"
./senechal-gw config check --strict >/dev/null 2>&1 && test_result "1.3 Strict mode" "pass" || test_result "1.3 Strict mode" "fail"

echo
echo "=== Results ==="
echo "Passed: $PASS"
echo "Failed: $FAIL"
echo "Known issues: $SKIP"

exit $FAIL
```

---

## Notes for Future Testing

### Test Environment Maintenance

1. **Before testing:**
   - Verify config.yaml is valid baseline
   - Clear any old job data if needed
   - Check gateway is not already running

2. **After testing:**
   - Restore config.yaml if modified
   - Kill gateway processes
   - Clean up temp files

### Pipeline Testing Limitations

Jobs created via `/trigger` endpoint don't have `event_context_id` set, which is required for `job inspect` lineage. To properly test:

1. Set up multi-hop pipeline with routing
2. Trigger jobs via configured entry points
3. Wait for pipeline completion
4. Inspect final job in chain

### Bug Tracking Integration

When filing bugs from test failures:
- Reference specific test suite/case number
- Include test output and expected vs actual behavior
- Note if regression of previously fixed bug
- Add test case number to bug card for traceability

---

**Maintained by:** @test-admin
**Version:** 1.0
**Last Test Run:** 2026-02-12
