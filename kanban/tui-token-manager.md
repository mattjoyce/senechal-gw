---
id: 40
status: backlog
priority: Low
blocked_by: [35, 36, 37, 38]
tags: [sprint-5, tui, security, ux, humans]
---

# TUI Token Manager (For Humans)

Interactive Bubble Tea interface for creating, viewing, and managing API tokens. Reads plugin manifests to present available scopes as checkboxes, generates secure tokens, and outputs config snippets to add to `config.yaml`.

**Note:** This TUI is for humans who prefer visual, interactive interfaces. For CLI-based workflows (scriptable, LLM-friendly), see card #38 (CLI Config Management). Both tools are complementary—use whichever fits your workflow.

## Motivation

**Current workflow for creating tokens:**
1. Manually edit `config.yaml`
2. Choose scope strings from docs/memory
3. Generate random token with `openssl rand -hex 32`
4. Paste into config
5. Run `ductile doctor` to validate
6. Restart service to apply

**Problems:**
- Easy to typo scope syntax (`withings:ro` vs `withings:r0`)
- Hard to remember which commands are read vs write
- No visibility into what existing tokens can do
- Manual token generation is tedious
- Risk of weak tokens if using simpler generation

**Solution:** Interactive TUI that:
- Shows discovered plugins and their commands (with types)
- Provides checkboxes to build scopes visually
- Generates cryptographically secure tokens
- Outputs ready-to-paste config YAML
- Can inspect existing tokens from current config

## Acceptance Criteria

- `ductile tokens` subcommand launches interactive TUI
- Views:
  - **List tokens** - Show existing tokens from config with decoded scopes
  - **Create token** - Interactive wizard with checkboxes for scopes
  - **Inspect token** - Show what a specific token can/cannot do
  - **Test token** - Make API call to verify token works
- Token creation wizard:
  - Name input
  - Plugin selection (checkboxes)
  - Per-plugin scope selection (ro/rw/specific commands)
  - System scope selection (read:*, admin:*)
  - Preview expanded scopes
  - Generate secure random token (32 bytes hex)
  - Output YAML snippet to append to config
- Token inspection:
  - Input token name or key
  - Show granted scopes
  - Show effective permissions (after expansion)
  - Show what API endpoints are accessible
- Keyboard navigation throughout
- No modifications to config.yaml (output only, user copies)

## UI Mockup

### Main Menu
```
┌─ Ductile Token Manager ──────────────────────────────────┐
│                                                             │
│  Discovered plugins: withings, garmin, echo, slack         │
│  Configured tokens: 4                                       │
│                                                             │
│  > List existing tokens                                     │
│    Create new token                                         │
│    Inspect token                                            │
│    Test token                                               │
│    Quit                                                     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### List Tokens View
```
┌─ Configured Tokens ───────────────────────────────────────┐
│                                                            │
│  admin-cli                                                 │
│    Scopes: read:*, trigger:*:*, admin:*                    │
│    Permissions: Full access                                │
│                                                            │
│  github-integration                                        │
│    Scopes: read:jobs, read:events, github-handler:rw      │
│    Permissions:                                            │
│      ✓ GET /job/{id}, GET /events                          │
│      ✓ POST /trigger/github-handler/* (all commands)      │
│      ✗ POST /trigger/withings/* (denied)                   │
│                                                            │
│  external-cron                                             │
│    Scopes: withings:ro                                     │
│    Permissions:                                            │
│      ✓ POST /trigger/withings/poll                         │
│      ✗ POST /trigger/withings/sync (type: write)          │
│                                                            │
│  [Enter] Inspect  [c] Create new  [q] Back                 │
└────────────────────────────────────────────────────────────┘
```

### Create Token Wizard - Step 1: Name
```
┌─ Create New Token ────────────────────────────────────────┐
│                                                            │
│  Token name (used in logs and config):                    │
│  > webhook-integration_                                    │
│                                                            │
│  [Enter] Next  [Esc] Cancel                                │
└────────────────────────────────────────────────────────────┘
```

### Create Token Wizard - Step 2: System Scopes
```
┌─ Create New Token: System Scopes ─────────────────────────┐
│                                                            │
│  Select system-level permissions:                         │
│                                                            │
│  [ ] read:*     - Read all system resources (jobs, etc.)  │
│  [ ] admin:*    - Admin operations (reload, reset)        │
│                                                            │
│  [Space] Toggle  [Enter] Next  [Esc] Back                 │
└────────────────────────────────────────────────────────────┘
```

### Create Token Wizard - Step 3: Plugin Scopes
```
┌─ Create New Token: Plugin Scopes ─────────────────────────┐
│                                                            │
│  Select plugins and access levels:                        │
│                                                            │
│  > [x] withings                                            │
│      (•) ro   - Read-only commands (poll)                 │
│      ( ) rw   - All commands (poll, sync, oauth_callback) │
│      ( ) deny - Explicitly deny                           │
│                                                            │
│    [x] garmin                                              │
│      ( ) ro   - Read-only commands (poll)                 │
│      (•) rw   - All commands (poll, sync)                 │
│      ( ) deny - Explicitly deny                           │
│                                                            │
│    [ ] echo                                                │
│                                                            │
│    [ ] slack                                               │
│                                                            │
│  [↑/↓] Navigate  [Space] Toggle  [Enter] Advanced         │
│  [n] Next  [b] Back  [Esc] Cancel                         │
└────────────────────────────────────────────────────────────┘
```

### Create Token Wizard - Advanced: Specific Commands
```
┌─ withings: Select Specific Commands ──────────────────────┐
│                                                            │
│  Choose individual commands (overrides ro/rw):            │
│                                                            │
│  [x] allow: poll                  (type: read)            │
│  [ ] allow: sync                  (type: write)           │
│  [ ] allow: oauth_callback        (type: write)           │
│                                                            │
│  Or use deny rules:                                       │
│  [ ] deny: sync                                           │
│                                                            │
│  [Space] Toggle  [Enter] Done  [Esc] Cancel               │
└────────────────────────────────────────────────────────────┘
```

### Create Token Wizard - Step 4: Preview & Generate
```
┌─ Create New Token: Preview ───────────────────────────────┐
│                                                            │
│  Token name: webhook-integration                          │
│                                                            │
│  Configured scopes:                                       │
│    - read:jobs                                            │
│    - read:events                                          │
│    - withings:ro                                          │
│    - garmin:rw                                            │
│                                                            │
│  Expanded permissions:                                    │
│    ✓ GET /job/{id}                                         │
│    ✓ GET /events                                           │
│    ✓ POST /trigger/withings/poll                          │
│    ✓ POST /trigger/garmin/poll                            │
│    ✓ POST /trigger/garmin/sync                            │
│    ✗ POST /trigger/withings/sync (type: write, denied)   │
│    ✗ POST /admin/* (admin:* not granted)                  │
│                                                            │
│  [g] Generate Token  [b] Back  [Esc] Cancel               │
└────────────────────────────────────────────────────────────┘
```

### Create Token Wizard - Final: Output
```
┌─ Token Created ───────────────────────────────────────────┐
│                                                            │
│  Generated token key (copy to environment):               │
│                                                            │
│  ┌────────────────────────────────────────────────────┐   │
│  │ a3f8c2d9e1b4567890abcdef12345678...                │   │
│  └────────────────────────────────────────────────────┘   │
│                                                            │
│  Add to config.yaml:                                       │
│                                                            │
│  ┌────────────────────────────────────────────────────┐   │
│  │ api:                                               │   │
│  │   auth:                                            │   │
│  │     tokens:                                        │   │
│  │       - key: ${WEBHOOK_INTEGRATION_TOKEN}          │   │
│  │         name: "webhook-integration"                │   │
│  │         scopes:                                    │   │
│  │           - "read:jobs"                            │   │
│  │           - "read:events"                          │   │
│  │           - "withings:ro"                          │   │
│  │           - "garmin:rw"                            │   │
│  └────────────────────────────────────────────────────┘   │
│                                                            │
│  Set environment variable:                                │
│  export WEBHOOK_INTEGRATION_TOKEN="a3f8c2d9e1..."         │
│                                                            │
│  [c] Copy token  [y] Copy YAML  [d] Done                  │
└────────────────────────────────────────────────────────────┘
```

## Implementation Details

**Package:** `internal/tui/tokens` or `cmd/ductile/tokens`

**Library:** [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Huh](https://github.com/charmbracelet/huh) for forms

**Main Model:**
```go
type Model struct {
    config         *config.Config
    pluginRegistry *plugin.Registry

    view           ViewMode  // list, create, inspect, test
    tokens         []config.Token

    // Creation wizard state
    wizardStep     int
    newToken       *TokenBuilder

    // UI state
    cursor         int
    selected       map[string]bool
}

type TokenBuilder struct {
    Name          string
    SystemScopes  []string           // read:*, admin:*
    PluginScopes  map[string]string  // plugin -> "ro"|"rw"|"deny"
    CommandScopes map[string][]string // plugin -> [allow:cmd, deny:cmd]
    GeneratedKey  string
}

func (tb *TokenBuilder) BuildScopes() []string {
    var scopes []string
    scopes = append(scopes, tb.SystemScopes...)

    for plugin, level := range tb.PluginScopes {
        if level != "" && level != "custom" {
            scopes = append(scopes, fmt.Sprintf("%s:%s", plugin, level))
        }
    }

    for plugin, cmds := range tb.CommandScopes {
        scopes = append(scopes, cmds...)
    }

    return scopes
}

func (tb *TokenBuilder) GenerateKey() string {
    bytes := make([]byte, 32)
    if _, err := rand.Read(bytes); err != nil {
        panic(err)  // Crypto failure, can't continue
    }
    return hex.EncodeToString(bytes)
}
```

**Scope Expansion (for preview):**
```go
func (m *Model) expandScopes(scopes []string) []Permission {
    var perms []Permission

    for _, scope := range scopes {
        // Use same expansion logic as token auth middleware
        expanded := expandScope(scope, m.pluginRegistry)
        perms = append(perms, expanded...)
    }

    return perms
}

type Permission struct {
    Endpoint string  // "GET /job/{id}", "POST /trigger/withings/poll"
    Allowed  bool    // true if granted, false if denied
    Reason   string  // "type: write, denied by ro scope"
}
```

**Token Inspection:**
```go
func (m *Model) inspectToken(tokenName string) *TokenInspection {
    // Find token in config
    var token *config.Token
    for _, t := range m.config.API.Auth.Tokens {
        if t.Name == tokenName {
            token = &t
            break
        }
    }

    if token == nil {
        return nil
    }

    // Expand scopes
    perms := m.expandScopes(token.Scopes)

    return &TokenInspection{
        Token:       *token,
        Permissions: perms,
    }
}
```

**YAML Generation:**
```go
func (tb *TokenBuilder) ToYAML() string {
    token := config.Token{
        Key:    fmt.Sprintf("${%s}", envVarName(tb.Name)),
        Name:   tb.Name,
        Scopes: tb.BuildScopes(),
    }

    // Marshal to YAML
    data, _ := yaml.Marshal([]config.Token{token})

    // Add context (assumes tokens array exists)
    return fmt.Sprintf(`# Add to api.auth.tokens:
      - %s`, string(data))
}

func envVarName(name string) string {
    // Convert "webhook-integration" -> "WEBHOOK_INTEGRATION_TOKEN"
    upper := strings.ToUpper(name)
    normalized := strings.ReplaceAll(upper, "-", "_")
    return normalized + "_TOKEN"
}
```

**Plugin Command Display:**
```go
func (m *Model) renderPluginCommands(pluginName string) string {
    plugin := m.pluginRegistry.Get(pluginName)
    if plugin == nil {
        return ""
    }

    var lines []string
    for cmd, meta := range plugin.Manifest.Commands {
        typeIcon := "📖" // read
        if meta.Type == "write" {
            typeIcon = "✏️" // write
        }
        lines = append(lines, fmt.Sprintf("  %s %s (type: %s)", typeIcon, cmd, meta.Type))
    }

    return strings.Join(lines, "\n")
}
```

**Clipboard Integration (optional):**
```go
import "github.com/atotto/clipboard"

func (m *Model) copyToClipboard(text string) error {
    return clipboard.WriteAll(text)
}
```

## CLI Integration

```go
var tokensCmd = &cobra.Command{
    Use:   "tokens",
    Short: "Manage API tokens interactively",
    RunE: func(cmd *cobra.Command, args []string) error {
        configPath, _ := cmd.Flags().GetString("config")

        // Load config
        cfg, err := config.Load(configPath)
        if err != nil {
            return err
        }

        // Discover plugins
        registry := plugin.NewRegistry()
        if err := registry.Discover(cfg.Service.PluginsDir); err != nil {
            return err
        }

        // Launch TUI
        p := tea.NewProgram(tokens.NewModel(cfg, registry))
        if _, err := p.Run(); err != nil {
            return err
        }

        return nil
    },
}

func init() {
    tokensCmd.Flags().String("config", "config.yaml", "Path to config file")
    rootCmd.AddCommand(tokensCmd)
}
```

## Testing

**Manual Testing:**
```bash
# Launch TUI
ductile tokens --config config.yaml

# Navigate through wizard
# Create token with:
#   - Name: test-integration
#   - Scopes: read:jobs, withings:ro
# Copy output to config

# Validate with doctor
ductile doctor --config config.yaml
# Should show new token is valid
```

**Unit Tests:**
```go
func TestTokenBuilderBuildScopes(t *testing.T) {
    tb := &TokenBuilder{
        SystemScopes: []string{"read:jobs"},
        PluginScopes: map[string]string{
            "withings": "ro",
            "garmin":   "rw",
        },
    }

    scopes := tb.BuildScopes()
    assert.ElementsMatch(t, []string{"read:jobs", "withings:ro", "garmin:rw"}, scopes)
}

func TestScopeExpansion(t *testing.T) {
    // Mock plugin registry with test plugins
    registry := &plugin.Registry{}
    registry.Register(mockWithingsPlugin())

    model := &Model{pluginRegistry: registry}
    perms := model.expandScopes([]string{"withings:ro"})

    assert.Len(t, perms, 1)
    assert.Equal(t, "POST /trigger/withings/poll", perms[0].Endpoint)
    assert.True(t, perms[0].Allowed)
}
```

## Dependencies

- Token scopes implementation (#35) - For scope syntax and expansion
- Manifest command type metadata (#36) - For displaying command types
- Config doctor (#37) - Can reuse scope validation logic
- Existing config loader (Sprint 1 ✓)
- Existing plugin discovery (Sprint 1 ✓)

**Go Dependencies:**
```go
require (
    github.com/charmbracelet/bubbletea v0.25.0
    github.com/charmbracelet/huh v0.3.0       // Form components
    github.com/charmbracelet/lipgloss v0.9.1
    github.com/atotto/clipboard v0.1.4        // Optional, for copy
)
```

## Deferred Features

**Not in v1:**
- Token revocation (just remove from config.yaml)
- Token rotation (generate new key, update env)
- Usage statistics (API call counts per token)
- Expiration dates (use external secret manager)
- Audit log viewer (when token was created, by whom)

## Use Cases

**1. Setting up GitHub webhook integration:**
```bash
ductile tokens
# Select "Create new token"
# Name: github-integration
# System scopes: read:jobs, read:events
# Plugin scopes: github-handler:rw
# Generate
# Copy YAML and token to config and env
```

**2. Debugging token permissions:**
```bash
ductile tokens
# Select "Inspect token"
# Enter name: github-integration
# View: Can access GET /job/{id}, cannot trigger withings
```

**3. Creating monitoring token:**
```bash
ductile tokens
# Create token with only read:* scope
# Use for Grafana/TUI
```

## Accessibility

- Full keyboard navigation (no mouse required)
- Vi-style keys supported (j/k for up/down)
- Clear instructions at bottom of each screen
- Color-blind friendly icons (not just color to convey meaning)

## Security Considerations

- Generated tokens use crypto/rand (cryptographically secure)
- Token keys never written to disk by TUI (output to terminal only)
- Config.yaml uses ${ENV_VAR} references, not plaintext tokens
- TUI reads config but never modifies it (user copies YAML manually)

## Narrative

The token manager solves the "I just want to give GitHub read-only access" problem. Instead of:
1. Reading docs to understand scope syntax
2. Manually writing `scopes: ["read:jobs", "read:events"]`
3. Running doctor to validate
4. Generating token with openssl
5. Pasting everything in the right place

You just:
1. Run `ductile tokens`
2. Follow the wizard
3. Copy/paste the output

The checkboxes ensure you can't typo a scope, and the preview shows exactly what the token can/cannot do before you commit to it. The manifest integration means you don't need to remember which commands are read-only—the plugin declares it, the TUI shows it.

**Priority:** Low. Nice quality-of-life improvement, but not essential. Implement after core functionality is stable (Sprint 5 or later). The config doctor (#37) is higher priority because it prevents errors, while this tool just makes correct usage easier.
