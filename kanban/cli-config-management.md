---
id: 38
status: done
priority: Normal
blocked_by: [35, 36, 37, 39]
tags: [sprint-4, cli, config, llm-friendly]
---

# CLI Config Management Tool

Implement `ductile config` subcommands for managing configuration files. LLM-friendly CLI for creating tokens, editing scopes, validating config, and inspecting runtime state. No interactive prompts—designed for scripting and agent automation.

**Note:** This CLI is for both humans (who prefer command-line workflows) AND LLMs (automation/skills). For humans who prefer visual, interactive interfaces, see card #40 (TUI Token Manager). Both tools operate on the same config files—use whichever fits your workflow.

## Motivation

**Primary use case:** LLMs modifying config via skills

**Requirements:**
- Non-interactive (CLI, not TUI)
- Scriptable and automatable
- Clear, parseable output (JSON option)
- Atomic file operations with backups
- Integration with config doctor for validation

**Anti-pattern:** Interactive wizards that require human input. LLMs can't use TUIs.

## Acceptance Criteria

- `ductile config` subcommands for all config operations
- All commands work with `--config-dir` flag
- JSON output mode for programmatic parsing
- Atomic file writes with backup (.bak files)
- Auto-runs `ductile doctor` after modifications
- Exit codes: 0=success, 1=error, 2=validation warnings
- Help text optimized for LLM consumption (structured, examples)

## Execution Plan (2026-02-14)

1. Add shared mutation helpers for `--config-dir` resolution, atomic write + `.bak`, and automatic `config check`.
2. Implement `config token` and `config scope` command groups with `--format human|json`.
3. Implement `config plugin`, `config route`, and `config webhook` command groups.
4. Implement `config init`, `config backup`, and `config restore`.
5. Expand CLI tests and run `go test ./...` after each phase.

## Command Reference

### Token Management

**Create token:**
```bash
# Create scope file and register token
ductile config token create \
  --name github-integration \
  --scopes "read:jobs,read:events,github-handler:rw" \
  --description "GitHub webhook integration"

# Output:
# Created: /Users/matt/.config/ductile/scopes/github-integration.json
# Updated: /Users/matt/.config/ductile/tokens.yaml
# Token key: a3f8c2d9e1b4567890abcdef12345678...
#
# Set environment variable:
#   export GITHUB_INTEGRATION_TOKEN="a3f8c2d9..."
#
# Validation: ✓ All checks passed
```

**Create from file:**
```bash
# Use existing scope JSON file
ductile config token create \
  --name webhook-integration \
  --scopes-file /tmp/scopes.json

# Or read from stdin
cat scopes.json | ductile config token create \
  --name webhook-integration \
  --scopes-file -
```

**List tokens:**
```bash
ductile config token list

# Output (human-readable):
# Tokens in /Users/matt/.config/ductile/tokens.yaml:
#
# admin-cli
#   Created: 2026-02-10T10:00:00Z
#   Scopes: read:*, trigger:*:*, admin:*
#   Description: Full admin access
#
# github-integration
#   Created: 2026-02-10T11:30:00Z
#   Scopes: read:jobs, read:events, github-handler:rw
#   Description: GitHub webhook integration

# JSON output:
ductile config token list --format json
# [
#   {
#     "name": "admin-cli",
#     "scopes_file": "scopes/admin-cli.json",
#     "scopes": ["read:*", "trigger:*:*", "admin:*"],
#     "created_at": "2026-02-10T10:00:00Z",
#     "description": "Full admin access"
#   }
# ]
```

**Inspect token:**
```bash
ductile config token inspect github-integration

# Output:
# Token: github-integration
# Created: 2026-02-10T11:30:00Z
# Scope file: scopes/github-integration.json
# Hash: blake3:b4e9d3c0f2a5678901bcdefg2345...
#
# Configured scopes:
#   - read:jobs
#   - read:events
#   - github-handler:rw
#
# Effective permissions (expanded):
#   ✓ GET /job/{id}
#   ✓ GET /events
#   ✓ POST /trigger/github-handler/handle
#   ✓ POST /trigger/github-handler/health
#   ✗ POST /trigger/withings/* (not granted)
#   ✗ POST /admin/* (not granted)

# JSON output:
ductile config token inspect github-integration --format json
```

**Rehash token:**
```bash
# Update hash after manual scope file edit
ductile config token rehash github-integration

# Output:
# Token: github-integration
# Scope file: /Users/matt/.config/ductile/scopes/github-integration.json
# Old hash: blake3:b4e9d3c0...
# New hash: blake3:c5f0e4d1...
#
# Updated: /Users/matt/.config/ductile/tokens.yaml
# Validation: ✓ All checks passed
```

**Delete token:**
```bash
ductile config token delete github-integration

# Output:
# Backup: /Users/matt/.config/ductile/tokens.yaml.bak
# Deleted: github-integration from tokens.yaml
# Scope file preserved: scopes/github-integration.json
#
# To delete scope file too:
#   rm /Users/matt/.config/ductile/scopes/github-integration.json
```

### Scope Management

**Edit scopes:**
```bash
# Add scope to existing token
ductile config scope add github-integration "withings:ro"

# Output:
# Token: github-integration
# Added scope: withings:ro
# Updated: scopes/github-integration.json
# Updated hash in tokens.yaml: blake3:...
# Validation: ✓ All checks passed

# Remove scope
ductile config scope remove github-integration "read:events"

# Replace all scopes
ductile config scope set github-integration "read:jobs,github-handler:rw"
```

**Validate scopes:**
```bash
# Check if scope is valid for discovered plugins
ductile config scope validate "withings:ro"

# Output:
# Scope: withings:ro
# Plugin: withings (found)
# Type: ro (read-only)
# Expands to:
#   - trigger:withings:poll
# ✓ Valid

# Invalid scope:
ductile config scope validate "nonexistent:ro"
# Output:
# Scope: nonexistent:ro
# ✗ Plugin 'nonexistent' not found in plugins directory
# Exit code: 1
```

### Plugin Management

**List plugins:**
```bash
ductile config plugin list

# Output:
# Discovered plugins in /opt/ductile/plugins:
#
# withings (configured)
#   Commands: poll (read), sync (write), oauth_callback (write)
#   Schedule: hourly (jitter: 5m)
#   Circuit breaker: 3 failures, 10m cooldown
#
# garmin (configured)
#   Commands: poll (read), sync (write)
#   Schedule: 2h (jitter: 10m)
#
# echo (not configured)
#   Commands: poll (read), health (read)

# JSON output:
ductile config plugin list --format json
```

**Show plugin config:**
```bash
ductile config plugin show withings

# Output (YAML):
# schedule:
#   every: hourly
#   jitter: 5m
# circuit_breaker:
#   threshold: 3
#   cooldown: 10m
# config:
#   client_id: ${WITHINGS_CLIENT_ID}
#   client_secret: ${WITHINGS_CLIENT_SECRET}
```

**Edit plugin config:**
```bash
# Modify schedule
ductile config plugin set withings schedule.every "2h"

# Output:
# Updated: plugins.yaml
# Plugin: withings
# Changed: schedule.every (hourly → 2h)
# Validation: ✓ All checks passed

# Set config value
ductile config plugin set withings config.client_id '${NEW_CLIENT_ID}'
```

### Route Management

**List routes:**
```bash
ductile config route list

# Output:
# Routes in /Users/matt/.config/ductile/routes.yaml:
#
# withings:weight_updated → garmin
# withings:weight_updated → slack
# garmin:activity_completed → slack
```

**Add route:**
```bash
ductile config route add \
  --from withings \
  --event weight_updated \
  --to healthkit

# Output:
# Added route: withings:weight_updated → healthkit
# Updated: routes.yaml
# Validation: ✓ All checks passed
```

**Remove route:**
```bash
ductile config route remove \
  --from withings \
  --event weight_updated \
  --to slack

# Output:
# Removed route: withings:weight_updated → slack
# Updated: routes.yaml
```

### Webhook Management

**List webhooks:**
```bash
ductile config webhook list

# Output:
# Webhooks in /Users/matt/.config/ductile/webhooks.yaml:
#
# github
#   Path: /webhook/github
#   Plugin: github-handler
#   Command: handle
#   Secret: ${GITHUB_WEBHOOK_SECRET}
#
# stripe
#   Path: /webhook/stripe
#   Plugin: stripe-handler
#   Command: handle
#   Secret: ${STRIPE_WEBHOOK_SECRET}
```

**Add webhook:**
```bash
ductile config webhook add \
  --name github \
  --path /webhook/github \
  --plugin github-handler \
  --secret '${GITHUB_WEBHOOK_SECRET}'

# Output:
# Added webhook: github
# Path: /webhook/github → github-handler:handle
# Updated: webhooks.yaml
# Validation: ✓ All checks passed
```

### General Config

**Show compiled config:**
```bash
# Show full runtime config (after compilation)
ductile config show

# Output: Full YAML of compiled RuntimeConfig

# Show specific section
ductile config show --section api
ductile config show --section plugins

# JSON output
ductile config show --format json
```

**Initialize config directory:**
```bash
# Create default config structure
ductile config init

# Output:
# Created: /Users/matt/.config/ductile/
# Created: /Users/matt/.config/ductile/config.yaml
# Created: /Users/matt/.config/ductile/plugins.yaml
# Created: /Users/matt/.config/ductile/routes.yaml
# Created: /Users/matt/.config/ductile/webhooks.yaml
# Created: /Users/matt/.config/ductile/tokens.yaml
# Created: /Users/matt/.config/ductile/scopes/
#
# Set file permissions:
#   chmod 700 /Users/matt/.config/ductile
#   chmod 600 /Users/matt/.config/ductile/*.yaml
#   chmod 700 /Users/matt/.config/ductile/scopes

# Custom location
ductile config init --config-dir /etc/ductile
```

**Backup config:**
```bash
ductile config backup

# Output:
# Created backup: /Users/matt/.config/ductile/backup-2026-02-10T14-30-00.tar.gz
# Includes: config.yaml, plugins.yaml, routes.yaml, webhooks.yaml, tokens.yaml, scopes/

# Restore from backup
ductile config restore backup-2026-02-10T14-30-00.tar.gz
```

## Implementation

**Package:** `cmd/ductile/config/` or standalone binary `cmd/ductile-config/`

**Decision:** Use `ductile config` subcommand (not separate binary). Keeps tooling unified.

### Token Create Implementation

```go
func runTokenCreate(cmd *cobra.Command, args []string) error {
    configDir := getConfigDir(cmd)
    name, _ := cmd.Flags().GetString("name")
    scopesStr, _ := cmd.Flags().GetString("scopes")
    scopesFile, _ := cmd.Flags().GetString("scopes-file")
    description, _ := cmd.Flags().GetString("description")

    // Parse scopes
    var scopes []string
    if scopesFile != "" {
        // Load from file or stdin
        data, err := readScopeFile(scopesFile)
        var scopeDef ScopeDefinition
        json.Unmarshal(data, &scopeDef)
        scopes = scopeDef.Scopes
    } else {
        scopes = strings.Split(scopesStr, ",")
    }

    // Create scope definition
    scopeDef := ScopeDefinition{
        Scopes: scopes,
        Metadata: map[string]interface{}{
            "description": description,
            "created_at":  time.Now().Format(time.RFC3339),
        },
    }

    // Write scope file
    scopeFilePath := filepath.Join(configDir, "scopes", name+".json")
    if err := writeScopeFile(scopeFilePath, scopeDef); err != nil {
        return err
    }
    fmt.Printf("Created: %s\n", scopeFilePath)

    // Compute hash
    hash := computeBLAKE3Hash(&scopeDef)

    // Generate token key
    tokenKey := generateSecureToken()

    // Create token entry
    token := Token{
        Name:        name,
        Key:         envVarRef(name),  // ${GITHUB_INTEGRATION_TOKEN}
        ScopesFile:  "scopes/" + name + ".json",
        ScopesHash:  hash,
        CreatedAt:   time.Now(),
        Description: description,
    }

    // Update tokens.yaml (with backup)
    tokensPath := filepath.Join(configDir, "tokens.yaml")
    if err := backupFile(tokensPath); err != nil {
        return err
    }

    if err := appendToken(tokensPath, token); err != nil {
        return err
    }
    fmt.Printf("Updated: %s\n", tokensPath)

    // Output token key
    fmt.Printf("Token key: %s\n\n", tokenKey)
    fmt.Printf("Set environment variable:\n")
    fmt.Printf("  export %s=\"%s\"\n\n", envVarName(name), tokenKey)

    // Run validation
    fmt.Printf("Running validation...\n")
    doctor := NewDoctor(configDir)
    result, _ := doctor.Validate(context.Background())

    if !result.Valid {
        fmt.Printf("✗ Validation failed:\n")
        for _, err := range result.Errors {
            fmt.Printf("  - %s\n", err.Message)
        }
        return cli.Exit("", 1)
    }

    fmt.Printf("✓ Validation passed\n")
    return nil
}

func generateSecureToken() string {
    bytes := make([]byte, 32)
    rand.Read(bytes)
    return hex.EncodeToString(bytes)
}

func envVarRef(name string) string {
    return fmt.Sprintf("${%s}", envVarName(name))
}

func envVarName(name string) string {
    // "github-integration" → "GITHUB_INTEGRATION_TOKEN"
    upper := strings.ToUpper(name)
    normalized := strings.ReplaceAll(upper, "-", "_")
    return normalized + "_TOKEN"
}

func backupFile(path string) error {
    if _, err := os.Stat(path); os.IsNotExist(err) {
        return nil  // File doesn't exist, no backup needed
    }

    backupPath := path + ".bak"
    return copyFile(path, backupPath)
}
```

## LLM Skill Integration

**Example skill:**
```yaml
# ~/.claude/skills/ductile/create-webhook-token.md
---
name: create-webhook-token
description: Create an API token for a webhook integration
---

Create a scoped API token for webhook integration.

Usage:
```bash
ductile config token create \
  --name {{token_name}} \
  --scopes "{{scopes}}" \
  --description "{{description}}"
```

The command will:
1. Create scope file in scopes/{{token_name}}.json
2. Update tokens.yaml with BLAKE3 hash
3. Generate secure random token
4. Validate configuration

After running, set the environment variable as instructed.
```

**LLM invocation:**
```
User: "Add a token for GitHub webhooks with read access to jobs"

LLM: I'll create a scoped token for GitHub webhooks.

[Uses create-webhook-token skill]
ductile config token create \
  --name github-webhooks \
  --scopes "read:jobs,read:events,github-handler:rw" \
  --description "GitHub webhook integration with read-only access"

[Output shown to user with next steps]
```

## Error Handling

**Clear, actionable errors:**

```bash
$ ductile config token create --name test --scopes "invalid:ro"

Error: Invalid scope 'invalid:ro'
  Plugin 'invalid' not found in /opt/ductile/plugins

Available plugins:
  - withings
  - garmin
  - echo
  - slack

Example valid scopes:
  - withings:ro     (read-only access)
  - garmin:rw       (full access)
  - echo:allow:poll (specific command)

Exit code: 1
```

## Output Formats

**Human-readable (default):**
- Colorized (if TTY)
- Clear headings
- Bullet points
- Examples in errors

**JSON (--format json):**
```json
{
  "status": "success",
  "token": {
    "name": "github-integration",
    "scopes_file": "scopes/github-integration.json",
    "scopes_hash": "blake3:...",
    "created_at": "2026-02-10T11:30:00Z"
  },
  "token_key": "a3f8c2d9e1b4567890abcdef...",
  "env_var": "GITHUB_INTEGRATION_TOKEN"
}
```

## Testing

**Unit Tests:**
```go
func TestTokenCreate(t *testing.T) {
    dir := t.TempDir()
    initConfigDir(t, dir)

    cmd := tokenCreateCmd()
    cmd.SetArgs([]string{
        "--config-dir", dir,
        "--name", "test-token",
        "--scopes", "read:jobs",
    })

    err := cmd.Execute()
    assert.NoError(t, err)

    // Verify scope file created
    scopeFile := filepath.Join(dir, "scopes/test-token.json")
    assert.FileExists(t, scopeFile)

    // Verify tokens.yaml updated
    tokens := loadTokens(t, dir)
    assert.Len(t, tokens, 1)
    assert.Equal(t, "test-token", tokens[0].Name)
}
```

**Integration Test:**
```bash
# Initialize config
ductile config init --config-dir /tmp/test-config

# Create token
ductile config token create \
  --config-dir /tmp/test-config \
  --name test \
  --scopes "read:jobs"

# Verify files exist
test -f /tmp/test-config/scopes/test.json
test -f /tmp/test-config/tokens.yaml

# Validate
ductile doctor --config-dir /tmp/test-config
# Exit code: 0
```

## Dependencies

- Multi-file config system (#39) - Operates on separate config files
- Token scopes (#35) - Creates scoped tokens
- Manifest metadata (#36) - Validates scope references
- Config doctor (#37) - Runs validation after changes

## Deferred Features

**Not in v1:**
- Interactive mode (intentionally avoided)
- Config diff viewer
- Undo/rollback commands
- Config templates
- Batch operations (create multiple tokens at once)

## Narrative

The CLI config tool is designed for automation-first workflows. Every command is non-interactive, scriptable, and LLM-friendly. When an agent needs to add a webhook integration, it can run `ductile config token create` with explicit arguments—no prompts, no interactivity, just clear input/output.

The atomic file operations (backup before modify) and automatic validation (doctor check after change) ensure safety. If something goes wrong, the .bak files provide rollback, and the exit codes tell the LLM whether it succeeded.

This is infrastructure for "conversational configuration"—where you tell an agent to add a GitHub webhook, and it safely modifies routes.yaml, webhooks.yaml, and tokens.yaml, validates everything, and reports back with the token to set in your environment.

**Priority:** Normal. Implement after multi-file config (#39) is stable. Not blocking for Sprint 3, but valuable for Sprint 4 operational workflows.

- 2026-02-14: Moved card to `doing` and split implementation into five phases to ship #38 incrementally with tests and frequent commits. (by @codex)
- 2026-02-14: Implemented `config token/scope/plugin/route/webhook/init/backup/restore` commands with `--config-dir`, JSON output, atomic file writes + `.bak`, checksum refresh, and post-change validation with 0/1/2 exit semantics. Added CLI tests for create/list/inspect flows, scope mutation, plugin/route/webhook commands, and init/backup/restore lifecycle. (by @codex)
- 2026-02-14: Verified `go test ./cmd/ductile` passes after changes. Repository-wide `go test ./...` still fails in unrelated packages due existing constructor signature drift in `internal/api`, `internal/dispatch`, `internal/e2e`, and `internal/scheduler` tests. (by @codex)
