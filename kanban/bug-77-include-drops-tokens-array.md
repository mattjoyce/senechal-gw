---
id: 77
status: done
priority: High
tags: [bug, config, include, auth, critical]
---

# BUG: include[] drops auth.tokens array during config merge

## Description

When using `include[]` to split config across files, the `auth.tokens` array is lost during merge. The merged config shows only `api_key: ""` instead of the tokens array defined in the included file.

## Impact

- **Severity**: HIGH - Authentication config silently dropped
- **Scope**: Config include mechanism, API auth
- **User Experience**: API auth doesn't work with split configs
- **Security**: Silent auth failure could lead to misconfiguration

## Evidence

### File Structure:
```
config-split.yaml:
  log_level: debug
  database: ...
  include:
    - config-parts/api.yaml

config-parts/api.yaml:
  api:
    enabled: true
    auth:
      tokens:
        - token: test_admin_token_local
          scopes: ["*"]
```

### Test Results:

**1. Source file is correct:**
```bash
$ cat config-parts/api.yaml
api:
  enabled: true
  listen: "localhost:8080"
  auth:
    tokens:
      - token: test_admin_token_local
        scopes: ["*"]
```

**2. Merged config DROPS the tokens array:**
```bash
$ ./senechal-gw config show --config config-split.yaml | grep -A5 "auth:"
auth:
    api_key: ""    # ← Only default field, tokens array missing!
```

**3. Validation detects the problem:**
```bash
$ ./senechal-gw config check --config config-split.yaml
Configuration valid (1 warning(s))
  WARN  [api] api.auth: API enabled but no authentication configured
```

The warning is CORRECT - auth IS missing from the merged config, even though it's defined in the included file!

## Root Cause

The config merge logic doesn't correctly handle arrays during `include[]` resolution. When merging:
- Scalar values work (listen: "localhost:8080" ✓)
- Nested objects partially work (auth: {} ✓)
- **Arrays are dropped** (tokens: [...] ✗)

Likely issue in `internal/config/loader.go` merge function.

## Expected Behavior

**After merging includes, the config should contain:**
```yaml
api:
  enabled: true
  listen: "localhost:8080"
  auth:
    api_key: ""
    tokens:              # ← Should be present!
      - token: test_admin_token_local
        scopes: ["*"]
```

## Reproduction

```bash
cd ~/admin/senechal-test

# Create split config (already done)
ls config-split.yaml config-parts/api.yaml

# Check source file has tokens
cat config-parts/api.yaml  # Shows tokens array ✓

# Check merged config
./senechal-gw config show --config config-split.yaml | grep -A10 "auth:"
# Result: tokens array missing ✗

# Validation warning
./senechal-gw config check --config config-split.yaml
# WARN: API enabled but no authentication configured ✓ (correctly detects missing auth)
```

## Testing Done

Created split config structure:
```
config-split.yaml        (root with include[])
config-parts/
  ├── api.yaml          (contains auth.tokens array)
  └── plugins.yaml      (contains plugins config)
```

**Results:**
- ✅ Include mechanism loads files
- ✅ Plugins merge correctly
- ❌ `auth.tokens` array dropped during merge
- ✅ Validation correctly warns about missing auth
- ❌ Silent failure - no error, just data loss

## Related Issues

- Card #76: Hardcoded filenames (config lock)
- Config include merging logic
- Array handling in YAML merge

## Suggested Investigation

Check `internal/config/loader.go` for:
1. How includes are resolved and merged
2. Array merge logic (append vs replace vs drop?)
3. Deep merge vs shallow merge behavior

**Suspected code area:**
- `loadIncludedFiles()` function
- YAML merge/override logic
- Struct merging (Go struct tags, omitempty behavior)

## Workaround

**Don't use includes for auth config** - keep tokens in main config file:

```yaml
# config.yaml (monolithic, works)
api:
  enabled: true
  auth:
    tokens:
      - token: test_token
```

## Security Implications

This is more than a convenience bug - it's a **silent security failure**:

1. Admin thinks auth is configured (it's in api.yaml)
2. Merged config silently drops tokens
3. API validation warns, but doesn't error
4. Gateway might start with no auth (depending on defaults)
5. No obvious error message explaining what happened

## Testing Recommendations

After fix:
1. ✅ Split config with tokens array - verify present in merged config
2. ✅ Split config with webhooks array - verify present
3. ✅ Split config with plugin array - verify present
4. ✅ Nested includes (A includes B, B includes C) - verify arrays preserved
5. ✅ Array append behavior - if main + include both have tokens, what happens?

## Narrative

- 2026-02-12: Discovered during config include testing per user request. Split test config into config-split.yaml + config-parts/api.yaml + config-parts/plugins.yaml. Config check succeeded but warned "API enabled but no authentication configured" despite tokens being defined in api.yaml. Investigation revealed tokens array is silently dropped during include merge. The merged config (via `config show`) only shows `api_key: ""`, not the tokens array. This is a HIGH severity bug because auth config is silently lost with no error - only a warning that's easy to miss. The validation is working correctly (detects missing auth), but the include merge is broken for arrays. (by @test-admin)
