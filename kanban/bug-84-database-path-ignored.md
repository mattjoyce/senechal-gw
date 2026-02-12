---
id: 84
status: todo
priority: Medium
tags: [bug, config, database, paths]
---

# BUG: Database path configuration ignored

## Description

The `database.path` configuration setting is ignored. Gateway always creates database at hardcoded `./data/state.db` regardless of config value.

## Impact

- **Severity**: Medium - Unexpected behavior, but workaround exists
- **Scope**: Database location and file naming
- **User Experience**: Config doesn't work as documented, creates surprise files

## Evidence

**Test**: TP-001 Schedule Trigger (2026-02-13)

**Config specified**:
```yaml
# config/senechal-gw/config.yaml
database:
  path: ./senechal-test/data/senechal.db
```

**Actual behavior**:
```json
{"level":"INFO","msg":"database opened","path":"./data/state.db"}
```

**File system**:
```bash
$ ls -la data/
total 304
-rw-r--r-- 1 matt matt   4096 state.db      # Created here
-rw-r--r-- 1 matt matt  32768 state.db-shm
-rw-r--r-- 1 matt matt 255472 state.db-wal

$ ls -la senechal-test/data/
total 332
-rw-r--r-- 1 matt matt      0 senechal.db   # Original (unused)
-rw-r--r-- 1 matt matt 307200 state.db      # From previous run
```

## Expected Behavior

**From CONFIG_SPEC.md**:
```yaml
database:
  path: ./data/senechal.db
```

Gateway should:
1. Respect the configured path
2. Create database at specified location
3. Create parent directories if needed (or fail with clear error)

## Root Cause (Suspected)

**Likely issues**:

1. **Config not loaded**: Database path defaulted before config parsed
   ```go
   // Might happen too early in startup
   dbPath := "./data/state.db"  // Hardcoded default
   config := loadConfig()       // Loads config after default set
   ```

2. **Wrong config key**: Code might look for different field name
   ```go
   // Code might expect:
   config.State.Path       // Instead of: config.Database.Path
   ```

3. **Filename override**: Path used but filename always `state.db`
   ```go
   dbPath := config.Database.Path
   dbFile := filepath.Join(filepath.Dir(dbPath), "state.db")  // Overrides filename
   ```

**Evidence for option 2**: CONFIG_SPEC.md shows:
```yaml
state:
  path: ./data/state.db
```

While I used:
```yaml
database:
  path: ./senechal-test/data/senechal.db
```

## Testing Recommendations

**Test 1: Verify correct config key**
```yaml
# Try "state" instead of "database"
state:
  path: ./senechal-test/data/state.db
```

**Test 2: Check startup logs**
```bash
./senechal-gw system start --config config.yaml -v 2>&1 | grep -i "database\|state"
# Look for path resolution logs
```

**Test 3: Check config parsing**
```bash
./senechal-gw config show 2>&1 | grep -A 2 "state:\|database:"
# See what config actually loaded
```

**After fix, verify**:
- [ ] Database created at configured path
- [ ] Parent directory auto-created if missing (or clear error)
- [ ] Filename from config respected
- [ ] Log shows correct path at startup
- [ ] Works with relative and absolute paths

## Related

- CONFIG_SPEC.md: Documents `state.path` (not `database.path`)
- USER_GUIDE.md: Example uses `state.path`
- TP-001: Discovered during initial UAT

## Workaround

Use the hardcoded location:
```yaml
state:
  path: ./data/state.db  # Probably works
```

Or run from directory where `./data/` is acceptable.

## Narrative

- 2026-02-13: Discovered during TP-001 setup. Configured database path as `database.path: ./senechal-test/data/senechal.db` to isolate test environment. Gateway started successfully but created database at `./data/state.db` instead. Log message confirmed: "database opened" at "./data/state.db". Checking CONFIG_SPEC.md, noticed docs show `state.path` not `database.path` - likely using wrong config key. Also note filename is always `state.db` even though config specified `senechal.db`. Medium priority since workaround exists (use hardcoded path), but breaks config-as-spec principle. (by @test-admin)
