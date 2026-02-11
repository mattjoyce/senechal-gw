---
id: 63
status: done
priority: Normal
blocked_by: []
tags: [bug, plugin, fabric, fixed]
---

# BUG: Fabric plugin fails to load - invalid command 'execute'

## Description

The fabric plugin in `plugins/fabric/` fails to load at startup with the following error:

```
invalid manifest: invalid command "execute" (valid: poll, handle, health, init)
```

## Details

**Error Source**: Plugin manifest validation during startup
**Location**: `plugins/fabric/manifest.yaml`
**Impact**: Fabric plugin is unavailable, but does not affect core gateway functionality

## Root Cause

The plugin manifest appears to be using an unsupported command name `execute` which is not recognized by the gateway. The valid command types according to the protocol are:
- `poll` - Scheduled data fetching
- `handle` - Event processing
- `health` - Health check
- `init` - One-time initialization

## Evidence

From Docker test environment logs (container: senechal-gw-test):
```
{"time":"2026-02-11T21:50:47.957855758Z","level":"WARN","msg":"failed to load plugin","component":"main","plugin":"fabric","error":"invalid manifest: invalid command \"execute\" (valid: poll, handle, health, init)"}
```

## Reproduction Steps

1. Start senechal-gw with default configuration
2. Check logs during plugin discovery phase
3. Observe warning message about fabric plugin failure

## Expected Behavior

The fabric plugin should load successfully with a valid command type that matches the gateway's protocol specification.

## Suggested Fix

Update the fabric plugin's `manifest.yaml` to use one of the valid command types (likely `poll` or `handle` depending on the plugin's intended purpose).

## Test Environment

- **Location**: `~/senechal-gw/`
- **Container**: senechal-gw-test
- **Config**: config.test.yaml
- **Gateway Version**: Latest from main branch

## Resolution

Fixed by updating `plugins/fabric/manifest.yaml`:

**Before:**
```yaml
commands:
  - name: execute
    type: write
  - name: health
    type: read
```

**After:**
```yaml
commands: [poll, health]
```

The manifest was using a complex structure with `name` and `type` fields, but the gateway expects a simple list of valid command names. Changed to use `poll` (for scheduled execution) and `health` (for health checks).

## Verification

After restart, logs show successful load:
```
{"time":"2026-02-11T22:00:08Z","level":"INFO","msg":"loaded plugin","plugin":"fabric","version":"0.1.0","protocol":1}
{"level":"INFO","msg":"plugin discovery complete","count":2}
```

## Narrative

- 2026-02-12: Discovered during initial test environment validation. The fabric plugin manifest uses 'execute' command which is not part of the v1 protocol specification. This prevents the plugin from loading, though it doesn't crash the gateway. (by @assistant/test-admin)
- 2026-02-12: Fixed by correcting the manifest format. The plugin now loads successfully. The manifest structure was incorrect - it used a list of objects with name/type fields instead of a simple list of command names. Changed to use valid commands [poll, health]. Verified successful load in logs. (by @assistant/test-admin)
