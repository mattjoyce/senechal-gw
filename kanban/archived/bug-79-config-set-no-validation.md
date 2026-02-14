---
id: 79
status: done
priority: High
tags: [bug, cli, config, validation, data-integrity]
---

# BUG: config set doesn't validate before writing

## Description

The `config set --apply` command writes values to config files without running validation first. This allows invalid configurations to be written, corrupting the config and preventing subsequent CLI operations.

## Impact

- **Severity**: High - Can corrupt config files
- **Scope**: Config integrity
- **User Experience**: Broken config, CLI commands fail

## Evidence

```bash
# Test: Set a value that creates invalid config
$ ./ductile config set --apply plugins.echo.enabled=true
Successfully set "plugins.echo.enabled" to "true"

# Config is now invalid (echo needs a schedule)
$ ./ductile config check
Load error: invalid configuration: plugin "echo": schedule is required for enabled plugins

# Cannot even read values anymore
$ ./ductile config get plugins.echo.enabled
Load error: invalid configuration: plugin "echo": schedule is required for enabled plugins

# Config is corrupted until manually fixed
```

## Expected Behavior

**Should validate BEFORE writing:**

```bash
# Attempt to set invalid value
$ ./ductile config set --apply plugins.echo.enabled=true

# Should run validation first
Validating configuration...
✗ Validation failed:
  - plugin "echo": schedule is required for enabled plugins

Error: Cannot apply change - would create invalid configuration
Exit code: 1

# Config file NOT modified (rollback/abort)
```

**Alternative: Validate AFTER writing (but before commit):**

```bash
$ ./ductile config set --apply plugins.echo.enabled=true

Writing: plugins.echo.enabled = true
Validating configuration...
✗ Validation failed: plugin "echo": schedule is required

Error: Configuration invalid after change
Action: Restored from backup (config.yaml.bak)
Exit code: 1
```

## Root Cause

The `config set` implementation writes values without calling the config validator:

**Likely code path:**
```go
func runConfigSet(path, value string, apply bool) error {
    // Load config
    cfg := loadConfig()

    // Set value
    setNestedValue(cfg, path, value)

    // Write config
    writeConfig(cfg)  // ← Writes without validation!

    fmt.Println("Successfully set...")
    return nil
}
```

**Missing validation:**
```go
func runConfigSet(path, value string, apply bool) error {
    cfg := loadConfig()
    setNestedValue(cfg, path, value)

    // SHOULD DO THIS:
    if err := validateConfig(cfg); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    writeConfig(cfg)
    return nil
}
```

## Combined Risk with Bug #78

This bug combines with bug #78 (no backups) to create a critical failure mode:

1. User runs `config set --apply plugins.echo.enabled=true`
2. No backup created (bug #78)
3. Invalid value written (bug #79)
4. Config corrupted
5. All CLI commands fail to load config
6. No automatic way to recover

**Recovery requires manual edit or git restore.**

## Reproduction

```bash
cd ~/admin/ductile-test

# Start with valid config
./ductile config check
# Output: Configuration valid.

# Set a value that creates invalid state
./ductile config set --apply plugins.echo.enabled=true
# Output: Successfully set...

# Config is now broken
./ductile config check
# Output: Load error: invalid configuration...

# Cannot use CLI anymore
./ductile config get service.log_level
# Output: Load error: invalid configuration...

# Manual recovery required
vi config.yaml  # Set echo.enabled back to false
```

## Suggested Fix

**Option 1: Validate before write (preferred)**
```go
func runConfigSet(path, value string, apply bool) error {
    cfg := loadConfig()

    // Create backup
    if apply {
        backupFile(configPath)
    }

    // Modify in memory
    setNestedValue(cfg, path, value)

    // Validate BEFORE writing
    if err := validateConfig(cfg); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // Write only if valid
    if apply {
        writeConfig(cfg)
        fmt.Println("Successfully set...")
    }

    return nil
}
```

**Option 2: Transaction with rollback**
```go
func runConfigSet(path, value string, apply bool) error {
    if apply {
        backupFile(configPath)
    }

    cfg := loadConfig()
    setNestedValue(cfg, path, value)

    if apply {
        writeConfig(cfg)

        // Validate after write
        if err := validateConfig(cfg); err != nil {
            // Rollback on validation failure
            restoreBackup(configPath)
            return fmt.Errorf("validation failed, restored backup: %w", err)
        }
    }

    return nil
}
```

## Testing Recommendations

After fix, verify:
1. ✅ Setting valid value succeeds
2. ✅ Setting invalid value is rejected BEFORE write
3. ✅ Config file not modified when validation fails
4. ✅ Error message explains why validation failed
5. ✅ Exit code non-zero on validation failure
6. ✅ Works with nested keys
7. ✅ Dry-run mode also validates

## Narrative

- 2026-02-12: Discovered during comprehensive CLI testing. Tested setting `plugins.echo.enabled=true` which requires a schedule field. The command succeeded with "Successfully set..." message, but config was invalid afterward. All subsequent CLI commands failed with "Load error: invalid configuration". Had to manually edit config.yaml to restore functionality. The `config set` command should run validation BEFORE writing to prevent this corruption scenario. Combined with bug #78 (no backups), this creates a high-severity data integrity issue. (by @test-admin)
- 2026-02-13: Fixed by making persisted `SetPath` changes transactional: write candidate config, immediately reload+validate full config, and automatically roll back file bytes on validation failure. `config set --apply` now rejects invalid mutations without leaving the config corrupted. Added regression tests in both config and CLI layers. (by @assistant)
