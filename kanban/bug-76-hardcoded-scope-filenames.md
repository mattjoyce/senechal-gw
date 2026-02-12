---
id: 76
status: todo
priority: Normal
tags: [bug, config, lock, technical-debt]
---

# BUG: config lock uses hardcoded filenames instead of discovering includes

## Description

The `config lock` command hardcodes specific filenames (`tokens.yaml`, `webhooks.yaml`) instead of dynamically discovering which files are actually included in the config. This is a remnant from the old fixed-filename model before the system moved to flexible includes.

## Impact

- **Severity**: Normal - Config lock works but shows misleading messages
- **Scope**: Config integrity system
- **User Experience**: Confusing "SKIP tokens.yaml: not found" messages for files that don't exist

## Evidence

```bash
$ ./senechal-gw config lock -v
Processing directory: /home/matt/admin/senechal-test
  SKIP tokens.yaml: not found (optional)
  SKIP webhooks.yaml: not found (optional)
```

**Problem:** My config doesn't use these files, yet the tool looks for them.

**Root cause:** Hardcoded filenames in `internal/config/loader.go`:

```go
// Line 173, 361:
if basename == "tokens.yaml" || basename == "webhooks.yaml" {
    // Treat as scope file
}
```

## Expected Behavior

**Correct flow:**
1. Parse the main config file
2. Discover ALL included files (via `include:` directives)
3. Lock only the files that are actually referenced
4. No hardcoded filenames

**Example:**
```yaml
# config.yaml
include:
  - auth/my-tokens.yaml
  - integrations/my-webhooks.yaml
  - custom-plugins.yaml
```

Should lock: `config.yaml`, `auth/my-tokens.yaml`, `integrations/my-webhooks.yaml`, `custom-plugins.yaml`

**NOT**: Look for hardcoded `tokens.yaml` and `webhooks.yaml`

## Current Implementation Issues

### 1. Hardcoded in loader.go (lines 173, 361)
```go
if basename == "tokens.yaml" || basename == "webhooks.yaml" {
    // Scope file detection by NAME, not by content/role
}
```

### 2. Hardcoded in lock command
The lock command passes these hardcoded names to the hash generator.

### 3. Tests reinforce the hardcoding
```go
// hash_test.go line 16:
GenerateChecksumsWithReport(tmpDir, []string{"tokens.yaml", "webhooks.yaml"}, true)
```

## Correct Approach

**Discover includes dynamically:**

```go
// Pseudo-code for correct behavior:
func discoverConfigFiles(mainConfigPath string) ([]string, error) {
    files := []string{mainConfigPath}

    // Parse main config
    cfg, err := parseConfig(mainConfigPath)
    if err != nil {
        return nil, err
    }

    // Add all included files
    for _, includePath := range cfg.Include {
        resolvedPath := resolvePath(includePath, configDir)
        files = append(files, resolvedPath)
    }

    return files, nil
}

// Then in lock command:
files, err := discoverConfigFiles("config.yaml")
GenerateChecksums(configDir, files)
```

**No hardcoded names!**

## Suggested Fix

1. **Remove hardcoded basename checks** in `loader.go`
2. **Add `discoverIncludedFiles()` function** that parses config and returns actual includes
3. **Update lock command** to use discovered files instead of hardcoded list
4. **Update tests** to test arbitrary filenames, not just tokens.yaml/webhooks.yaml

### Example fix for loader.go:

```go
// BEFORE (wrong):
if basename == "tokens.yaml" || basename == "webhooks.yaml" {
    // scope file
}

// AFTER (correct):
func isScopeFile(path string, mainConfig *Config) bool {
    // Check if this file contains sensitive data based on CONTENT
    // OR: mark scope files explicitly in config
    // NOT: check filename
}
```

## Related

- Migration from fixed filenames to flexible includes
- Config integrity verification system
- `config lock` command implementation

## Reproduction

```bash
cd ~/admin/senechal-test

# Create config WITHOUT tokens.yaml or webhooks.yaml
cat > config.yaml << EOF
database:
  path: ./data/state.db
plugins:
  echo:
    enabled: true
EOF

# Run config lock
./senechal-gw config lock -v

# WRONG: Shows "SKIP tokens.yaml" even though it's not referenced
# RIGHT: Should only process config.yaml
```

## Workaround

None needed - the command still works, just shows misleading messages.

## Testing Recommendations

After fix, verify:
1. ✅ Config with `include: [auth.yaml]` locks config.yaml + auth.yaml
2. ✅ Config with `include: [a.yaml, b.yaml, c.yaml]` locks all 4 files
3. ✅ Config with no includes only locks config.yaml
4. ✅ No "SKIP tokens.yaml" messages for configs that don't use it
5. ✅ Arbitrary filenames work (not just tokens/webhooks)

## Narrative

- 2026-02-12: Discovered during CLI testing. User ran `config lock -v` and saw misleading "SKIP tokens.yaml: not found (optional)" messages for files not referenced in their config. Investigation revealed hardcoded filenames in loader.go (lines 173, 361) and lock command. This is technical debt from the migration to flexible includes. The system should dynamically discover included files from the config, not hardcode specific filenames. User correctly identified: "build a monolithic config, then process. always." - parse the config tree, discover all includes, then lock those files. No assumptions about filenames. (by @test-admin)
