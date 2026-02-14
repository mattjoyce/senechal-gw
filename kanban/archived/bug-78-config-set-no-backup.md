---
id: 78
status: cancelled
priority: High
tags: [bug, cli, config, safety, data-loss]
---

# CANCELLED: config set doesn't create backup before modifying

## Description

The `config set --apply` command modifies config files without creating `.bak` backup files first. This violates the safety requirement from card #38 and creates risk of data loss when changes go wrong.

## Impact

- **Severity**: High - Risk of data loss
- **Scope**: Config modification safety
- **User Experience**: Cannot rollback bad changes

## Evidence

```bash
# Test: Modify config and check for backup
$ ls *.bak
# No backup files exist

$ ./ductile config set --apply plugins.echo.enabled=true
Successfully set "plugins.echo.enabled" to "true"

$ ls *.bak
# Still no backup created!

$ ls -la config.yaml*
-rw-rw-r-- 1 matt matt 625 Feb 12 20:28 config.yaml
# No config.yaml.bak file
```

## Expected Behavior

**From card #38 specification:**
- Create `.bak` file before ANY modification
- Atomic file operations with backups
- Enable rollback for bad changes

**Expected flow:**
1. Read current config.yaml
2. Create config.yaml.bak (copy of current)
3. Write new values to config.yaml
4. Report success with backup location

**Example output:**
```bash
$ ductile config set --apply plugins.echo.enabled=true
Backup: /path/to/config.yaml.bak
Successfully set "plugins.echo.enabled" to "true"
```

## Root Cause

The `config set` implementation doesn't call backup creation before writing.

**Likely location:** `cmd/ductile/config.go` or similar, missing:
```go
func setConfigValue(path, value string) error {
    // MISSING: Create backup first!
    // backupFile(configPath)

    // Modify config
    // Write config
}
```

## Related Issue

This combines with bug #79 (no validation before write) to create a dangerous scenario:
1. No backup created
2. Invalid value written
3. Config corrupted
4. No way to rollback

## Reproduction

```bash
cd ~/admin/ductile-test

# Verify no backups exist
ls *.bak 2>&1

# Modify config
./ductile config set --apply plugins.echo.enabled=false

# Check for backup
ls *.bak 2>&1
# Expected: config.yaml.bak
# Actual: No such file or directory

# Verify config was modified
grep "echo:" -A1 config.yaml
```

## Testing Recommendations

After fix, verify:
1. ✅ `.bak` file created before first modification
2. ✅ Each modification updates the backup timestamp
3. ✅ Backup contains the pre-modification content
4. ✅ User can manually restore from backup
5. ✅ Works with nested keys (plugins.x.y.z)

## Narrative

- 2026-02-12: Discovered during comprehensive CLI testing. The `config set --apply` command successfully modifies config files but doesn't create safety backups first. Tested by setting plugins.echo.enabled=true and checking for config.yaml.bak - no backup file created. This violates the card #38 specification requirement for "atomic file operations with backups". Combined with bug #79 (no validation), this creates a data loss risk when invalid values corrupt the config. (by @test-admin)

## Cancellation Note

- 2026-02-12: **CANCELLED** - No design spec requires backups for current `config set` implementation. Card #38 (which mentions backups) is in BACKLOG and describes a different, more extensive CLI system. Current `config set` is simpler and doesn't need to follow card #38 requirements. Bug #79 (validation) remains valid as config corruption is a real issue. (by @test-admin)
