---
id: 75
status: todo
priority: High
tags: [testing, ci-cd, quality, review, strategy]
---

# REVIEW: Testing Strategy & Coverage Analysis

Comprehensive assessment of current testing practices, gaps, and recommendations for improving quality gates.

## Executive Summary

**Current State: Tests exist but run manually**

✅ **What's Working:**
- 27 test files with good unit test coverage (44-86%)
- All tests passing
- E2E test framework exists
- Core packages well-tested (API 79%, Protocol 86%, Plugin 80%)

❌ **Critical Gaps:**
- **NO CI/CD pipeline** - tests only run manually
- **NO pre-commit hooks** - devs can push broken code
- **NO automated E2E testing** - manual testing required
- **CLI coverage 9.6%** - command-line bugs slip through
- **NO plugin testing** - plugins untested by devs

## Current Test Coverage Analysis

```
Package                Coverage    Assessment
---------------------------------------------------
cmd/ductile        9.6%       ❌ CRITICAL - CLI untested
internal/api           79.1%      ✅ Good
internal/config        70.4%      ✅ Good
internal/dispatch      60.9%      ⚠️  Adequate
internal/doctor        78.3%      ✅ Good
internal/protocol      86.1%      ✅ Excellent
internal/plugin        80.0%      ✅ Good
internal/router/dsl    84.8%      ✅ Excellent
internal/queue         48.5%      ⚠️  Low
internal/lock          44.1%      ⚠️  Low
internal/webhook       44.6%      ⚠️  Low
```

**Average Coverage: ~65%** (excluding main CLI)

## What Tests Exist

### ✅ Unit Tests (27 files)
- `internal/api/*_test.go` - API endpoints, auth, handlers
- `internal/protocol/codec_test.go` - Plugin protocol encoding
- `internal/router/dsl/*_test.go` - Pipeline DSL compilation
- `internal/dispatch/dispatcher_test.go` - Job execution
- `internal/config/*_test.go` - Config validation, loading
- `internal/state/*_test.go` - Database operations
- `internal/workspace/*_test.go` - Workspace management

### ✅ Integration Tests
- `internal/api/integration_test.go` - API with real DB
- `internal/e2e/pipeline_test.go` - Multi-hop pipeline routing
- `internal/e2e/echo_plugin_test.go` - Plugin execution

### ❌ What's Missing

#### 1. **NO CI/CD Pipeline**
**Impact: CRITICAL**

```bash
$ ls .github/workflows/
# No CI workflows found
```

**What this means:**
- Devs don't know if their changes break tests
- No automated testing before merge
- I'm catching bugs that should fail in CI
- No test coverage enforcement
- No build verification

**Examples of bugs that should've been caught:**
- Schedule validation bug (card #74) - unit test should catch
- Missing strings import (card #62) - build should fail
- Field priority bug (card #73) - unit test should catch

#### 2. **NO Pre-Commit Hooks**
**Impact: HIGH**

```bash
$ ls .git/hooks/
# Default hooks only, none active
```

**What's needed:**
- `go fmt` check
- `go vet` check
- Run tests before commit
- Build verification
- Linting (golangci-lint)

#### 3. **CLI Testing Gap (9.6% coverage)**
**Impact: HIGH**

The main CLI (`cmd/ductile/main.go`) has only 9.6% test coverage, yet this is what users and LLM operators interact with!

**Bugs found in CLI:**
- `config check` schedule validation false warnings
- `job inspect --json` flag not working
- `config set` command broken
- No `--help` at action level

**All could've been caught with CLI tests.**

#### 4. **NO Plugin Testing**
**Impact: MEDIUM-HIGH**

```bash
$ find plugins/ -name "*test*"
# No plugin tests
```

**Plugins are untested:**
- `fabric/run.py` - 171 lines, no tests
- `file_handler/run.py` - 409 lines, no tests
- `echo/run.sh` - no tests

**Issues found:**
- Fabric manifest format errors (card #63)
- Fabric command mismatch (card #66)
- Field priority bug (card #73)

**Should have:**
- Unit tests for plugin logic
- Integration tests with gateway
- Manifest validation tests

#### 5. **NO Automated E2E Pipeline Tests**
**Impact: MEDIUM**

E2E test framework exists (`internal/e2e/`) but:
- Not run automatically
- No CI integration
- Manual testing required
- No smoke tests for common flows

**What's needed:**
- Automated 3-hop pipeline test
- API trigger → plugin → routing → completion
- Baggage propagation validation
- Workspace isolation tests

## Recommended Testing Strategy

### Phase 1: CI/CD Foundation (CRITICAL)

Create `.github/workflows/ci.yml`:

```yaml
name: CI
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Install dependencies
        run: |
          go mod download
          sudo apt-get install -y sqlite3

      - name: Verify formatting
        run: |
          if [ "$(gofmt -s -l . | wc -l)" -gt 0 ]; then
            echo "Code not formatted. Run: gofmt -s -w ."
            exit 1
          fi

      - name: Vet
        run: go vet ./...

      - name: Run tests
        run: go test -v -race -coverprofile=coverage.txt ./...

      - name: Check coverage
        run: |
          coverage=$(go tool cover -func=coverage.txt | grep total | awk '{print $3}' | sed 's/%//')
          echo "Coverage: ${coverage}%"
          if (( $(echo "$coverage < 60.0" | bc -l) )); then
            echo "Coverage below 60%"
            exit 1
          fi

      - name: Build
        run: CGO_ENABLED=1 go build ./cmd/ductile

      - name: CLI smoke test
        run: |
          ./ductile version
          ./ductile config check --config test-fixtures/valid-config.yaml

  e2e:
    runs-on: ubuntu-latest
    needs: test
    steps:
      - uses: actions/checkout@v4
      - name: Run E2E tests
        run: go test -v ./internal/e2e/...
```

**Benefits:**
- Catches build failures before merge
- Enforces test coverage minimums
- Validates all PRs automatically
- Prevents regressions

### Phase 2: Pre-Commit Hooks

Create `.git/hooks/pre-commit`:

```bash
#!/bin/bash
set -e

echo "Running pre-commit checks..."

# Format check
if [ "$(gofmt -s -l . | wc -l)" -gt 0 ]; then
  echo "❌ Code not formatted. Running gofmt..."
  gofmt -s -w .
  git add -u
fi

# Vet
echo "Running go vet..."
go vet ./...

# Fast tests only (skip E2E)
echo "Running unit tests..."
go test -short ./...

echo "✅ Pre-commit checks passed"
```

### Phase 3: CLI Test Coverage

Create `cmd/ductile/cli_test.go`:

```go
func TestConfigCheckCommand(t *testing.T) {
    tests := []struct {
        name       string
        args       []string
        wantExit   int
        wantOutput string
    }{
        {
            name:       "valid config",
            args:       []string{"config", "check", "-config", "testdata/valid.yaml"},
            wantExit:   0,
            wantOutput: "Configuration valid",
        },
        {
            name:       "invalid config",
            args:       []string{"config", "check", "-config", "testdata/invalid.yaml"},
            wantExit:   1,
            wantOutput: "Config load error",
        },
        {
            name:       "json output",
            args:       []string{"config", "check", "-format", "json"},
            wantExit:   0,
            wantOutput: `{"valid":true}`,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test CLI execution
        })
    }
}

func TestJobInspectJSON(t *testing.T) {
    // Test that --json actually produces JSON
    output := runCLI("job", "inspect", testJobID, "--json")

    var result map[string]any
    if err := json.Unmarshal([]byte(output), &result); err != nil {
        t.Fatalf("--json output is not valid JSON: %v", err)
    }

    if result["job_id"] != testJobID {
        t.Errorf("job_id mismatch")
    }
}
```

**Target: 80%+ CLI coverage**

### Phase 4: Plugin Testing

Create `plugins/file_handler/run_test.py`:

```python
import unittest
import json
import sys
from io import StringIO
from run import handle_command, read_file, write_file

class TestFileHandler(unittest.TestCase):
    def test_read_file_success(self):
        config = {"allowed_read_paths": "/tmp"}
        payload = {"file_path": "/tmp/test.txt"}
        # ... test logic

    def test_write_prioritizes_result_over_content(self):
        """Regression test for card #73"""
        payload = {
            "content": "original",
            "result": "processed",
            "output_dir": "/tmp"
        }
        response = write_file(config, {}, payload)
        # Verify 'result' was written, not 'content'

    def test_path_security_validation(self):
        """Test realpath-based security checks"""
        config = {"allowed_write_paths": "/tmp"}
        payload = {
            "content": "test",
            "output_path": "/tmp/../etc/passwd"  # Attack!
        }
        response = write_file(config, {}, payload)
        self.assertEqual(response["status"], "error")

if __name__ == '__main__':
    unittest.main()
```

**Run in CI:**
```yaml
- name: Test plugins
  run: |
    cd plugins/file_handler && python3 -m pytest run_test.py
    cd plugins/fabric && python3 -m pytest run_test.py
```

### Phase 5: Automated E2E Smoke Tests

Add to CI:

```yaml
smoke-test:
  runs-on: ubuntu-latest
  steps:
    - name: Start gateway
      run: |
        ./ductile system start --config test-config.yaml &
        sleep 3

    - name: Test API trigger
      run: |
        curl -X POST http://localhost:8080/trigger/echo/handle \
          -H "Authorization: Bearer test-token" \
          -d '{"payload": {"message": "test"}}'

    - name: Verify job completion
      run: |
        # Poll for job completion
        # Verify events emitted
        # Check workspace artifacts
```

## Testing Maturity Model

**Current Level: 2 (Managed) - "Tests exist but run manually"**

**Target Level: 4 (Quantitatively Managed) - "Metrics-driven quality"**

### Level Progression:

1. **Initial** - No formal testing
2. **Managed** ← **WE ARE HERE**
   - Tests exist
   - Run manually by devs
   - Some coverage metrics
   - No automation

3. **Defined** ← **TARGET**
   - CI/CD pipeline
   - Automated test runs
   - Coverage requirements
   - Pre-commit hooks

4. **Quantitatively Managed**
   - Coverage trends tracked
   - Quality metrics dashboards
   - Regression detection
   - Performance benchmarks

5. **Optimizing**
   - Continuous improvement
   - Automated test generation
   - Predictive quality analysis

## My Role as Test Admin

### What I Should Focus On:

✅ **Exploratory Testing**
- Real-world usage scenarios
- Edge cases and integration issues
- UX and LLM operator experience
- Multi-component interaction bugs

✅ **System Testing**
- Full pipeline E2E flows
- Performance and load testing
- Security testing
- Documentation validation

✅ **Test Strategy**
- Identify gaps in automated testing
- Design comprehensive test scenarios
- Create test data and fixtures
- Report systemic issues

### What Devs Should Own:

❌ **Unit Tests** - Devs write alongside code
❌ **Integration Tests** - Devs test component interactions
❌ **Regression Tests** - Devs add tests for fixed bugs
❌ **Build Validation** - CI catches before reaching me

## Bugs That Should've Been Caught Earlier

### By Unit Tests:
- **Card #73**: Field priority bug (plugins/file_handler/run.py)
  - Simple unit test: pass both fields, verify priority
- **Card #74**: Schedule validation bug (internal/doctor/doctor.go)
  - Unit test: validate "daily" doesn't trigger warning

### By CI/CD:
- **Build failure**: Missing strings import (card #62 pre-fix)
  - `go build` in CI would've caught
- **Plugin manifest format** (card #63)
  - Manifest validation tests in CI

### By Pre-Commit Hooks:
- **Formatting issues**
- **Vet warnings**
- **Obvious syntax errors**

## Recommendations Summary

### Immediate (Week 1):
1. ✅ Create `.github/workflows/ci.yml` with test automation
2. ✅ Add pre-commit hook template to repo
3. ✅ Create CLI test suite (cmd/ductile/cli_test.go)
4. ✅ Document test requirements in CONTRIBUTING.md

### Short Term (Week 2-3):
1. ✅ Add plugin unit tests (pytest for Python plugins)
2. ✅ Increase coverage requirement to 70%
3. ✅ Add E2E smoke tests to CI
4. ✅ Create test fixtures directory

### Medium Term (Month 1):
1. ✅ Add coverage trending
2. ✅ Integration with codecov.io or similar
3. ✅ Automated performance benchmarks
4. ✅ Security scanning (gosec, bandit for Python)

### Long Term (Ongoing):
1. ✅ Test-driven development culture
2. ✅ Regression test for every bug fix
3. ✅ Quality metrics dashboard
4. ✅ Automated test generation for DSL validation

## Conclusion

**The devs ARE writing tests** (27 test files, 65% average coverage) but **tests only run manually**. This creates a gap where:

1. ✅ Core logic is tested (API, protocol, router)
2. ❌ Tests don't run before merge (no CI)
3. ❌ CLI is undertested (9.6%)
4. ❌ Plugins are untested (0%)
5. ❌ E2E flows are manual

**My role should be:**
- Exploratory testing of real-world scenarios
- System-level integration testing
- UX/operator experience validation
- Identifying testing strategy gaps

**NOT:**
- Catching bugs that unit tests should find
- Manual regression testing
- Build validation
- Basic functionality checks

**Fix: Add CI/CD pipeline and the test coverage is already good enough to catch most issues automatically.**

## Narrative

- 2026-02-12: Conducted comprehensive testing strategy review. Found 27 test files with 65% average coverage - good unit test foundation. However, CRITICAL gap: no CI/CD pipeline means tests only run manually. Devs can push code without running tests. Additional gaps: CLI coverage only 9.6%, no plugin tests, no automated E2E testing. Many bugs I found (schedule validation, field priority, CLI flags) should've been caught by automated tests in CI. Recommendation: implement GitHub Actions CI/CD as highest priority. This will shift my role from catching basic unit test issues to higher-value exploratory and system testing. Created detailed implementation plan with 5 phases: CI/CD, pre-commit hooks, CLI tests, plugin tests, E2E automation. (by @test-admin)
