---
id: 36
status: done
priority: High
blocked_by: []
tags: [sprint-3, plugins, manifest, security]
---

# Manifest Command Type Metadata (Read vs Write)

Extend plugin manifest schema to declare which commands are read-only (safe, idempotent) vs read-write (side effects). Enables manifest-driven token scopes like `plugin:ro` and `plugin:rw`.

## Motivation

**Current state:** Manifest declares command names but not semantics:
```yaml
commands: [poll, sync, oauth_callback]
```

**Problem:** Token scopes (#35) can't distinguish safe operations from dangerous ones without hardcoding knowledge of each plugin.

**Solution:** Plugins declare their own security model:
```yaml
commands:
  poll:
    type: read
    description: "Fetch latest measurements"
  sync:
    type: write
    description: "Push data to Withings API"
```

**Benefit:** Token scopes can use `withings:ro` instead of manually listing `["trigger:withings:poll"]`.

## Acceptance Criteria

- Manifest schema supports both old (array) and new (object) command formats
- New format: `commands.<name>.type` with values: `read` or `write`
- New format: `commands.<name>.description` (optional, for documentation)
- Manifest validation enforces: type is required if using object format
- Plugin registry provides helper: `GetCommandsByType(pluginName, "read") -> []string`
- Backward compatible: Array format still works, treats all commands as `type: write` (safe default)
- Validation fails if `type` is neither `read` nor `write`

## Schema Changes

### Old Format (Still Supported)
```yaml
name: echo
protocol: 1
entrypoint: run.sh
commands: [poll, health]  # All treated as type: write
config_keys: []
```

### New Format (Recommended)
```yaml
name: withings
protocol: 1
entrypoint: run.py
commands:
  poll:
    type: read
    description: "Fetch latest measurements from Withings API"
  sync:
    type: write
    description: "Push weight data to Withings API"
  oauth_callback:
    type: write
    description: "Handle OAuth2 callback and store tokens"
config_keys: [client_id, client_secret]
```

### Mixed (Also Valid)
```yaml
# Can use object format even without type metadata
commands:
  poll:
    description: "Fetch data"
  sync:
    description: "Push data"
# Both treated as type: write (safe default)
```

## Command Type Semantics

### `type: read` (Safe Operations)
**Characteristics:**
- No external side effects (doesn't modify remote state)
- Idempotent (can be called multiple times safely)
- May update local plugin state (e.g., `last_poll_at`)
- Examples: `poll`, `fetch`, `get`, `list`, `health`

**What you CAN do in a read command:**
- Fetch data from external APIs
- Update plugin state (e.g., cache, timestamps)
- Emit events based on fetched data

**What you CANNOT do:**
- POST/PUT/DELETE to external APIs
- Send messages/notifications
- Trigger webhooks
- Delete or modify remote resources

### `type: write` (Side Effects)
**Characteristics:**
- Modifies external state
- May not be idempotent
- Use for OAuth callbacks, syncs, notifications
- Examples: `sync`, `send`, `notify`, `oauth_callback`, `delete`

**Default:** If type is not specified, assume `write` (paranoid default).

## Implementation Details

**Package:** `internal/plugin` (extend existing manifest types)

**Manifest Types (manifest.go):**
```go
type Manifest struct {
    Name        string
    Protocol    int
    Entrypoint  string
    Commands    CommandMap  // Changed from []string
    ConfigKeys  []string
}

// CommandMap supports both old (array) and new (object) formats
type CommandMap map[string]CommandMetadata

func (cm CommandMap) UnmarshalYAML(value *yaml.Node) error {
    // If array format: ["poll", "sync"]
    if value.Kind == yaml.SequenceNode {
        var cmdList []string
        if err := value.Decode(&cmdList); err != nil {
            return err
        }
        for _, cmd := range cmdList {
            cm[cmd] = CommandMetadata{Type: "write"}  // Safe default
        }
        return nil
    }

    // If object format: {poll: {type: read, description: "..."}}
    var cmdMap map[string]CommandMetadata
    if err := value.Decode(&cmdMap); err != nil {
        return err
    }
    for name, meta := range cmdMap {
        // Default to write if type not specified
        if meta.Type == "" {
            meta.Type = "write"
        }
        // Validate type
        if meta.Type != "read" && meta.Type != "write" {
            return fmt.Errorf("invalid command type %q for %q (must be read or write)", meta.Type, name)
        }
        cm[name] = meta
    }
    return nil
}

type CommandMetadata struct {
    Type        string  `yaml:"type"`         // "read" or "write"
    Description string  `yaml:"description"`  // Optional human-readable description
}
```

**Plugin Methods (discovery.go):**
```go
// Existing method (unchanged)
func (p *Plugin) SupportsCommand(cmd string) bool {
    _, ok := p.Manifest.Commands[cmd]
    return ok
}

// New methods
func (p *Plugin) GetCommandType(cmd string) string {
    if meta, ok := p.Manifest.Commands[cmd]; ok {
        return meta.Type
    }
    return ""  // Command doesn't exist
}

func (p *Plugin) GetReadCommands() []string {
    return p.getCommandsByType("read")
}

func (p *Plugin) GetWriteCommands() []string {
    return p.getCommandsByType("write")
}

func (p *Plugin) getCommandsByType(typ string) []string {
    var commands []string
    for name, meta := range p.Manifest.Commands {
        if meta.Type == typ {
            commands = append(commands, name)
        }
    }
    sort.Strings(commands)
    return commands
}
```

**Registry Methods (discovery.go):**
```go
// Helper for scope expansion
func (r *Registry) GetCommandsByType(pluginName, typ string) ([]string, error) {
    plugin := r.Get(pluginName)
    if plugin == nil {
        return nil, fmt.Errorf("plugin not found: %s", pluginName)
    }
    return plugin.getCommandsByType(typ), nil
}
```

## Integration with Token Scopes (#35)

**Scope Expansion Logic (in internal/api):**
```go
// When parsing scope "withings:ro"
func (s *Server) expandScope(scope string) ([]string, error) {
    parts := strings.Split(scope, ":")

    // Manifest-driven shorthand
    if len(parts) == 2 {
        plugin, mode := parts[0], parts[1]

        switch mode {
        case "ro":
            cmds, err := s.pluginRegistry.GetCommandsByType(plugin, "read")
            if err != nil {
                return nil, err
            }
            // Expand to: ["trigger:withings:poll"]
            return expandToTriggerScopes(plugin, cmds), nil

        case "rw":
            p := s.pluginRegistry.Get(plugin)
            if p == nil {
                return nil, fmt.Errorf("plugin not found: %s", plugin)
            }
            // Expand to all commands: ["trigger:withings:poll", "trigger:withings:sync", ...]
            allCmds := make([]string, 0, len(p.Manifest.Commands))
            for cmd := range p.Manifest.Commands {
                allCmds = append(allCmds, cmd)
            }
            return expandToTriggerScopes(plugin, allCmds), nil
        }
    }

    // Low-level scope (action:resource:command)
    return []string{scope}, nil
}
```

## Migration Path

**Phase 1 (Sprint 3):** Add support, keep backward compatible
- Array format still works (all commands treated as `type: write`)
- Update docs to recommend new format for new plugins

**Phase 2 (Sprint 4+):** Migrate existing plugins
- Update echo, withings, garmin plugin manifests to use new format
- Classify commands appropriately

**Phase 3 (Future):** Deprecate array format
- Log warnings for array format
- Eventually require object format (major version bump)

## Testing

**Unit Tests (manifest_test.go):**
```go
func TestCommandMapUnmarshal(t *testing.T) {
    tests := []struct {
        name     string
        yaml     string
        expected CommandMap
    }{
        {
            name: "array format (backward compat)",
            yaml: "commands: [poll, sync]",
            expected: CommandMap{
                "poll": {Type: "write"},
                "sync": {Type: "write"},
            },
        },
        {
            name: "object format with types",
            yaml: `commands:
  poll:
    type: read
    description: "Fetch data"
  sync:
    type: write`,
            expected: CommandMap{
                "poll": {Type: "read", Description: "Fetch data"},
                "sync": {Type: "write"},
            },
        },
        {
            name: "object format without types (defaults to write)",
            yaml: `commands:
  poll:
    description: "Fetch data"`,
            expected: CommandMap{
                "poll": {Type: "write", Description: "Fetch data"},
            },
        },
        {
            name: "invalid type",
            yaml: `commands:
  poll:
    type: invalid`,
            expectErr: true,
        },
    }
    // ...
}

func TestPluginGetCommandsByType(t *testing.T) {
    plugin := &Plugin{
        Manifest: Manifest{
            Commands: CommandMap{
                "poll":   {Type: "read"},
                "sync":   {Type: "write"},
                "health": {Type: "read"},
            },
        },
    }

    readCmds := plugin.GetReadCommands()
    assert.Equal(t, []string{"health", "poll"}, readCmds)

    writeCmds := plugin.GetWriteCommands()
    assert.Equal(t, []string{"sync"}, writeCmds)
}
```

**Integration Test:**
```go
// Load plugin with new manifest format
// Verify scope expansion works correctly
func TestScopeExpansionWithManifestTypes(t *testing.T) {
    // Create plugin with read/write commands
    // Expand "withings:ro" scope
    // Assert only read commands are granted
}
```

## Example Plugin Manifests

**Echo Plugin (simple, all read):**
```yaml
name: echo
protocol: 1
entrypoint: run.sh
commands:
  poll:
    type: read
    description: "Echo back the current timestamp"
  health:
    type: read
    description: "Health check endpoint"
config_keys: []
```

**Withings Plugin (mixed):**
```yaml
name: withings
protocol: 1
entrypoint: run.py
commands:
  poll:
    type: read
    description: "Fetch latest weight/BP measurements from Withings API"
  sync:
    type: write
    description: "Push weight data to Withings API (if bidirectional sync enabled)"
  oauth_callback:
    type: write
    description: "Handle OAuth2 callback and persist access/refresh tokens to state"
config_keys:
  - client_id
  - client_secret
  - redirect_uri
```

**Slack Plugin (all write):**
```yaml
name: slack
protocol: 1
entrypoint: run.py
commands:
  notify:
    type: write
    description: "Send notification to configured Slack channel"
  react:
    type: write
    description: "Add emoji reaction to a message"
config_keys:
  - webhook_url
  - default_channel
```

## Documentation Updates

Update SPEC.md §6.1 (Manifest Format):
```markdown
### Command Metadata (Protocol v1)

Commands can be declared as array (legacy) or object (recommended):

**Array format:**
```yaml
commands: [poll, sync, health]
```

**Object format:**
```yaml
commands:
  poll:
    type: read
    description: "Fetch data from external API"
  sync:
    type: write
    description: "Push data to external API"
```

**Command types:**
- `read` - Idempotent, no external side effects (safe for automated retries)
- `write` - May modify external state (use with caution)

Default: `write` if type not specified (paranoid default).
```

## Dependencies

- Existing plugin discovery (#12) ✓
- Token scopes implementation (#35) - uses this for scope expansion

**Sequencing:** Implement alongside or before #35 (token scopes). Scopes can work without this (using low-level syntax), but ro/rw shorthands require manifest metadata.

## Deferred Features

**Not in v1:**
- Additional type values (e.g., `dangerous`, `admin`)
- Command parameter schemas (for validation)
- Required vs optional config_keys metadata
- Command-level rate limits in manifest

## Narrative

This enhancement solves the "how does a token know what's safe?" problem. Without it, granting a monitoring service access to your gateway requires either:
1. Manually listing every safe command in scope config (brittle, duplicates manifest)
2. Granting full access and hoping they don't misuse it

With command type metadata, plugins declare their own security boundaries, and token scopes reference that declaration. The implementation is backward compatible (array format still works) and the migration path is gradual (update plugins one by one).

**Priority:** High. Prerequisite for manifest-driven token scopes (#35), which is prerequisite for safe webhook integrations (Sprint 3).
