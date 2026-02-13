---
id: 39
status: done
priority: High
blocked_by: []
tags: [sprint-3, config, architecture]
---

# Multi-File Config System (Nagios-Style)

Implement multi-file configuration system using `~/.config/ductile/` with separate definition files. Config loader compiles into monolithic runtime config with preflight validation. BLAKE3 hashes ensure token scope file integrity.

## Motivation

**Current assumption:** Single `config.yaml` with everything inline.

**Problems:**
- Monolithic file hard to manage
- Token scopes mixed with service config
- No integrity checking on sensitive files
- Poor separation of concerns
- Difficult for LLMs to edit specific sections safely

**Solution:** Separate config files (Nagios approach):
- `config.yaml` - Service settings
- `plugins.yaml` - Plugin definitions
- `routes.yaml` - Event routing
- `webhooks.yaml` - Webhook endpoints
- `tokens.yaml` - Token registry (with BLAKE3 hashes)
- `scopes/*.json` - Token scope definitions

Compile at runtime with preflight validation.

## Config Directory Structure

```
~/.config/ductile/           # Default XDG location
├── config.yaml                  # Service-level settings
├── plugins.yaml                 # Plugin configurations & schedules
├── routes.yaml                  # Event routing rules
├── webhooks.yaml                # Webhook endpoint definitions
├── tokens.yaml                  # Token registry (BLAKE3 hashes)
└── scopes/
    ├── admin-cli.json          # Scope definition per token
    ├── github-integration.json
    └── webhook-integration.json
```

**Custom location via flag:**
```bash
ductile start --config-dir /etc/ductile/
ductile start --config-dir ./config-dev/
```

## File Formats

### config.yaml (Service Settings)
```yaml
service:
  plugins_dir: /opt/ductile/plugins
  tick_interval: 60s
  dedupe_ttl: 24h
  events:
    enabled: true
    buffer_size: 100

api:
  enabled: true
  listen: localhost:8080
```

### plugins.yaml (Plugin Definitions)
```yaml
plugins:
  withings:
    schedule:
      every: hourly
      jitter: 5m
    circuit_breaker:
      threshold: 3
      cooldown: 10m
    config:
      client_id: ${WITHINGS_CLIENT_ID}
      client_secret: ${WITHINGS_CLIENT_SECRET}

  garmin:
    schedule:
      every: 2h
      jitter: 10m
    config:
      api_key: ${GARMIN_API_KEY}
```

### routes.yaml (Event Routing)
```yaml
routes:
  - from: withings
    event_type: weight_updated
    to: garmin

  - from: withings
    event_type: weight_updated
    to: slack
```

### webhooks.yaml (Webhook Endpoints)
```yaml
webhooks:
  - name: github
    path: /webhook/github
    secret: ${GITHUB_WEBHOOK_SECRET}
    plugin: github-handler
    command: handle

  - name: stripe
    path: /webhook/stripe
    secret: ${STRIPE_WEBHOOK_SECRET}
    plugin: stripe-handler
    command: handle
```

### tokens.yaml (Token Registry with BLAKE3)
```yaml
tokens:
  - name: admin-cli
    key: ${ADMIN_API_KEY}
    scopes_file: scopes/admin-cli.json
    scopes_hash: blake3:a3f8c2d9e1b4567890abcdef1234567890abcdef1234567890abcdef12345678
    created_at: 2026-02-10T10:00:00Z
    description: "Full admin access"

  - name: github-integration
    key: ${GITHUB_API_KEY}
    scopes_file: scopes/github-integration.json
    scopes_hash: blake3:b4e9d3c0f2a5678901bcdefg2345678901bcdefg2345678901bcdefg23456789
    created_at: 2026-02-10T11:30:00Z
    description: "GitHub webhook integration"
```

### scopes/github-integration.json
```json
{
  "scopes": [
    "read:jobs",
    "read:events",
    "github-handler:rw"
  ],
  "metadata": {
    "description": "GitHub webhook integration",
    "created_at": "2026-02-10T11:30:00Z",
    "purpose": "Allow GitHub to query job status and trigger handlers"
  }
}
```

## Acceptance Criteria

**Config Loading:**
- Loader discovers config directory via `--config-dir` flag, `$DUCTILE_CONFIG_DIR` env, or `~/.config/ductile/`
- Loads all YAML files (config, plugins, routes, webhooks, tokens)
- Each file optional except `config.yaml` (others default to empty)
- Loads referenced scope JSON files
- Computes BLAKE3 hash of each scope file
- **Hard fails if hash mismatch** with detailed error message
- Interpolates environment variables across all files
- Compiles into single `RuntimeConfig` struct

**Validation:**
- Validates each file independently
- Cross-file reference validation:
  - Routes reference valid plugins (from plugins.yaml)
  - Webhooks reference valid plugins
  - Token scopes reference valid plugins/commands (from manifests)
  - Scope files exist and parse correctly
- Detects circular dependencies in routes
- All validation errors reported with file path and line number if possible

**Hash Integrity:**
- BLAKE3 hash computed over canonical JSON (sorted keys)
- Mismatch error shows:
  - Token name
  - Scope file path
  - Expected vs actual hash
  - Recovery instructions (rehash command)
- No fallback or auto-recompute (security: intentional hard fail)

**CLI Integration:**
- All commands default to `~/.config/ductile/`
- `--config-dir` flag overrides for all commands
- Works with: `start`, `doctor`, `config`, `tokens`, etc.

## Config Loader Implementation

**Package:** `internal/config`

```go
type Loader struct {
    configDir string
}

func NewLoader(configDir string) *Loader {
    if configDir == "" {
        configDir = defaultConfigDir()  // ~/.config/ductile
    }
    return &Loader{configDir: configDir}
}

func (l *Loader) Load(ctx context.Context) (*RuntimeConfig, error) {
    // 1. Load individual files
    svc, err := l.loadFile("config.yaml", &ServiceConfig{})
    if err != nil {
        return nil, fmt.Errorf("config.yaml: %w", err)
    }

    plugins, _ := l.loadFile("plugins.yaml", &PluginsConfig{})
    routes, _ := l.loadFile("routes.yaml", &RoutesConfig{})
    webhooks, _ := l.loadFile("webhooks.yaml", &WebhooksConfig{})
    tokens, _ := l.loadFile("tokens.yaml", &TokensConfig{})

    // 2. Load and verify scope files
    for i, token := range tokens.Tokens {
        scopes, err := l.loadScopeFile(token.ScopesFile)
        if err != nil {
            return nil, fmt.Errorf("token %q: %w", token.Name, err)
        }

        // Verify BLAKE3 hash
        actualHash := computeBLAKE3Hash(scopes)
        if actualHash != token.ScopesHash {
            return nil, &HashMismatchError{
                TokenName:    token.Name,
                ScopeFile:    filepath.Join(l.configDir, token.ScopesFile),
                ExpectedHash: token.ScopesHash,
                ActualHash:   actualHash,
            }
        }

        tokens.Tokens[i].Scopes = scopes.Scopes
    }

    // 3. Interpolate env vars
    runtime := &RuntimeConfig{
        Service:  svc,
        Plugins:  plugins.Plugins,
        Routes:   routes.Routes,
        Webhooks: webhooks.Webhooks,
        Tokens:   tokens.Tokens,
    }

    if err := l.interpolateEnvVars(runtime); err != nil {
        return nil, err
    }

    // 4. Validate cross-references
    if err := l.validateReferences(runtime); err != nil {
        return nil, err
    }

    return runtime, nil
}

func (l *Loader) loadFile(filename string, target interface{}) error {
    path := filepath.Join(l.configDir, filename)

    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) && filename != "config.yaml" {
            return nil  // Optional file
        }
        return err
    }

    return yaml.Unmarshal(data, target)
}

func (l *Loader) loadScopeFile(scopeFile string) (*ScopeDefinition, error) {
    path := filepath.Join(l.configDir, scopeFile)

    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("scope file not found: %s", path)
    }

    var scope ScopeDefinition
    if err := json.Unmarshal(data, &scope); err != nil {
        return nil, fmt.Errorf("invalid scope JSON: %w", err)
    }

    return &scope, nil
}

func computeBLAKE3Hash(scope *ScopeDefinition) string {
    // Canonical JSON (sorted keys) for deterministic hashing
    jsonBytes, _ := json.Marshal(scope)
    hash := blake3.Sum256(jsonBytes)
    return "blake3:" + hex.EncodeToString(hash[:])
}

func (l *Loader) validateReferences(runtime *RuntimeConfig) error {
    // Check routes reference valid plugins
    for _, route := range runtime.Routes {
        if _, exists := runtime.Plugins[route.From]; !exists {
            return fmt.Errorf("routes.yaml: source plugin %q not found in plugins.yaml", route.From)
        }
        if _, exists := runtime.Plugins[route.To]; !exists {
            return fmt.Errorf("routes.yaml: target plugin %q not found in plugins.yaml", route.To)
        }
    }

    // Check webhooks reference valid plugins
    for _, webhook := range runtime.Webhooks {
        if _, exists := runtime.Plugins[webhook.Plugin]; !exists {
            return fmt.Errorf("webhooks.yaml: plugin %q not found in plugins.yaml", webhook.Plugin)
        }
    }

    return nil
}
```

## Hash Mismatch Error UX

```go
type HashMismatchError struct {
    TokenName    string
    ScopeFile    string
    ExpectedHash string
    ActualHash   string
}

func (e *HashMismatchError) Error() string {
    return fmt.Sprintf(`Token scope integrity check failed

Token:        %s
Scope file:   %s
Expected:     %s
Actual:       %s

The scope file has been modified since it was registered in tokens.yaml.

To fix this issue:

  1. Review the scope file changes:
     cat %s

  2. If changes are intentional, update the hash:
     ductile config token rehash %s

  3. Or restore from backup:
     cp %s.bak %s

For security: Ductile will NOT start with mismatched hashes.`,
        e.TokenName,
        e.ScopeFile,
        e.ExpectedHash,
        e.ActualHash,
        e.ScopeFile,
        e.TokenName,
        e.ScopeFile, e.ScopeFile,
    )
}
```

## BLAKE3 Hash Library

**Dependency:**
```go
require (
    github.com/zeebo/blake3 v0.2.3
)
```

**Why BLAKE3:**
- Faster than SHA256
- Cryptographically secure
- Simple API
- Well-maintained Go library

## Backward Compatibility

**Sprint 3:** Support both formats
- Inline scopes in tokens.yaml (deprecated, logs warning)
- Scope files with hashes (new format)

**Sprint 4:** Escalate deprecation warnings
- Log at WARN level on every startup
- Include migration guide URL

**Sprint 5:** Remove inline scope support
- Breaking change, major version bump
- Document in release notes

**Migration path:**
```go
// Detect old format
if token.Scopes != nil && token.ScopesFile == "" {
    logger.Warn("Token uses deprecated inline scopes",
        "token", token.Name,
        "action", "migrate to scope files before Sprint 5",
        "docs", "https://docs.ductile.dev/migration/scopes")
}
```

## File Permissions Recommendations

```bash
# Config directory
chmod 700 ~/.config/ductile/

# Main config (readable by owner only)
chmod 600 ~/.config/ductile/config.yaml
chmod 600 ~/.config/ductile/plugins.yaml
chmod 600 ~/.config/ductile/routes.yaml
chmod 600 ~/.config/ductile/webhooks.yaml

# Token registry (sensitive)
chmod 600 ~/.config/ductile/tokens.yaml

# Scope files (read-only after creation)
chmod 400 ~/.config/ductile/scopes/*.json
```

## CLI Helper Commands

See card #38 (revised) for `ductile config` subcommands.

## Testing

**Unit Tests:**
```go
func TestConfigLoader(t *testing.T) {
    // Setup temp config dir
    dir := t.TempDir()
    writeFile(t, dir, "config.yaml", validServiceConfig)
    writeFile(t, dir, "plugins.yaml", validPluginsConfig)
    writeFile(t, dir, "tokens.yaml", validTokensConfig)
    writeFile(t, dir, "scopes/test.json", validScopeJSON)

    loader := NewLoader(dir)
    runtime, err := loader.Load(context.Background())

    assert.NoError(t, err)
    assert.Equal(t, 60*time.Second, runtime.Service.TickInterval)
    assert.Len(t, runtime.Tokens, 1)
}

func TestHashMismatch(t *testing.T) {
    dir := t.TempDir()
    // Write token with hash X
    writeFile(t, dir, "tokens.yaml", tokenWithHashX)
    // Write scope file that hashes to Y
    writeFile(t, dir, "scopes/test.json", scopeThatHashesToY)

    loader := NewLoader(dir)
    _, err := loader.Load(context.Background())

    assert.Error(t, err)
    assert.IsType(t, &HashMismatchError{}, err)
}
```

**Integration Test:**
```bash
# Create config directory
mkdir -p /tmp/ductile-test/{scopes}

# Write valid configs
cat > /tmp/ductile-test/config.yaml <<EOF
service:
  plugins_dir: ./plugins
  tick_interval: 60s
api:
  enabled: true
  listen: localhost:8080
EOF

# Start service
./ductile start --config-dir /tmp/ductile-test
# Should start successfully

# Tamper with scope file
echo '{"scopes":["admin:*"]}' > /tmp/ductile-test/scopes/test.json

# Restart service
./ductile start --config-dir /tmp/ductile-test
# Should fail with hash mismatch error
```

## Dependencies

- Existing config loader (Sprint 1) - Refactor to multi-file
- BLAKE3 library (new dependency)
- Token scopes (#35) - Uses scope files
- Manifest metadata (#36) - For scope validation

**Sequencing:** Implement in Sprint 3 alongside token scopes and webhooks.

## Benefits

**1. Separation of Concerns:**
- Each file focused on one aspect
- Easy to understand/modify specific sections

**2. LLM-Friendly:**
- LLMs can edit specific files safely
- Clear boundaries reduce risk of corruption
- Skills can target specific config aspects

**3. Security:**
- BLAKE3 hashes detect tampering
- Different file permissions per sensitivity
- Token scopes separated from main config

**4. Collaboration:**
- Multiple people/agents can own different files
- Version control shows focused changes
- Merge conflicts less likely

**5. Validation:**
- Compile-time cross-reference checking
- Fail fast with clear errors
- Preflight check before starting

## Deferred Features

**Not in v1:**
- Config file encryption at rest
- Git-based version control integration
- Config change auditing/history
- Hot reload of individual files
- Config templates/inheritance

## Narrative

The multi-file approach transforms config management from "edit one giant YAML carefully" to "edit focused files safely." The Nagios parallel is intentional—that model has proven itself for 20+ years of operational use. BLAKE3 hashes on scope files prevent accidental (or malicious) modifications from going unnoticed, which is critical when distributing tokens to external services.

The LLM-first design means config changes can be automated via skills—an LLM can confidently edit `routes.yaml` without touching `tokens.yaml`, and the preflight check catches any mistakes before runtime. This is the foundation for "conversational configuration" where you ask an agent to add a webhook integration and it safely modifies the right files.

**Priority:** High. Foundation for Sprint 3 (webhooks, tokens). Implement early in Sprint 3 before other features build on it.

---

## Status Updates

- 2026-02-10: Multi-file config system implemented. Config directory loading, BLAKE3 hash verification, cross-file reference validation all working. Tests passing. Ready for webhook and token scope integration. (by @claude)
