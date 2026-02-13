---
id: 37
status: done
priority: Normal
blocked_by: []
assignee: "@claude"
tags: [sprint-4, cli, config, validation]
---

# Config Doctor / Validation Tool

Implement `ductile doctor` command to validate configuration files before runtime. Catches errors early, validates scope references, checks plugin availability, and warns about common misconfigurations.

## Motivation

**Current state:** Config errors discovered at runtime when service starts or when a job fails.

**Problems:**
- Invalid plugin references silently ignored until scheduled
- Malformed scope syntax causes 403s without clear reason
- Webhook path conflicts discovered only when request arrives
- Missing env vars cause panic at startup
- No way to test config changes without restarting service

**Solution:** Pre-flight validation tool that checks config against discovered plugins and manifests.

## Acceptance Criteria

- `ductile doctor [--config path]` subcommand validates configuration
- Exit code 0 if valid, 1 if errors found, 2 if warnings only
- Checks:
  - YAML syntax valid
  - Required fields present (service.plugins_dir, api.listen, etc.)
  - Plugin references exist (plugins in config are discovered)
  - Token scopes reference valid plugins/commands/types
  - Webhook paths don't conflict
  - Route source plugins exist and emit referenced event types
  - No circular dependencies in routing (A → B → A)
  - Manifest metadata valid (type: read|write)
- Warnings (non-fatal):
  - Environment variables referenced but not set
  - Unused plugins (in plugins_dir but not in config)
  - Deprecated config syntax (old api_key vs tokens array)
  - Suspicious scheduler intervals (< 1m or > 24h)
- Colorized output: ✓ green checks, ✗ red errors, ⚠ yellow warnings
- `--strict` flag treats warnings as errors

## Command Interface

```bash
# Validate default config
ductile doctor

# Validate specific config
ductile doctor --config /path/to/config.yaml

# Strict mode (warnings = errors)
ductile doctor --strict

# JSON output (for CI/CD)
ductile doctor --format json
```

## Output Format

### Success (Exit 0)
```
✓ YAML syntax valid
✓ Service configuration valid
  - plugins_dir: ./plugins (3 plugins discovered)
  - tick_interval: 60s
✓ API configuration valid
  - listen: localhost:8080
  - 4 tokens configured
✓ Token scopes valid
  - admin-cli: read:*, trigger:*:*, admin:*
  - github-integration: read:jobs, read:events, github-handler:rw
  - external-cron: withings:ro (expands to: trigger:withings:poll)
  - oauth-callback: withings:allow:oauth_callback, garmin:allow:oauth_callback
✓ Plugins referenced in config
  - withings: 3 commands (2 read, 1 write)
  - garmin: 2 commands (1 read, 1 write)
  - echo: 2 commands (2 read)
✓ Webhooks configuration valid
  - /webhook/github → github-handler:handle
✓ Routes configuration valid
  - withings:weight_updated → garmin:handle, slack:notify
⚠ Warnings:
  - Plugin 'slack' discovered but not used in config
  - Environment variable ${GITHUB_WEBHOOK_SECRET} not set

Configuration valid (2 warnings)
```

### Errors (Exit 1)
```
✓ YAML syntax valid
✗ Token scopes invalid
  - Token 'github-integration' scope 'withings:invalid': invalid type (must be ro, rw, allow:cmd, or deny:cmd)
  - Token 'external-cron' scope 'nonexistent:ro': plugin 'nonexistent' not found
✗ Plugin references invalid
  - Plugin 'slack' referenced in routes but not discovered
✗ Webhooks configuration invalid
  - Path '/webhook/github' and '/webhook/github/' conflict (trailing slash ambiguity)

Configuration invalid (3 errors, 0 warnings)
```

### JSON Output (for CI/CD)
```json
{
  "valid": false,
  "errors": [
    {
      "category": "token_scopes",
      "message": "Token 'github-integration' scope 'withings:invalid': invalid type",
      "field": "api.auth.tokens[1].scopes[2]"
    }
  ],
  "warnings": [
    {
      "category": "env_vars",
      "message": "Environment variable ${GITHUB_WEBHOOK_SECRET} not set",
      "field": "webhooks[0].secret"
    }
  ]
}
```

## Implementation Details

**Package:** `internal/doctor` or `internal/validation`

**Main Validator:**
```go
type ValidationResult struct {
    Valid    bool
    Errors   []ValidationError
    Warnings []ValidationWarning
}

type ValidationError struct {
    Category string  // "yaml", "token_scopes", "plugin_refs", etc.
    Message  string
    Field    string  // JSON path: "api.auth.tokens[1].scopes[2]"
}

type ValidationWarning struct {
    Category string
    Message  string
    Field    string
}

type Doctor struct {
    config         *config.Config
    pluginRegistry *plugin.Registry
    logger         *slog.Logger
}

func (d *Doctor) Validate(ctx context.Context) (*ValidationResult, error) {
    result := &ValidationResult{Valid: true}

    // Run all checks
    d.validateYAMLSyntax(result)
    d.validateServiceConfig(result)
    d.validateAPIConfig(result)
    d.validateTokenScopes(result)
    d.validatePluginReferences(result)
    d.validateWebhooks(result)
    d.validateRoutes(result)
    d.warnUnusedPlugins(result)
    d.warnMissingEnvVars(result)
    d.warnDeprecatedSyntax(result)

    result.Valid = len(result.Errors) == 0
    return result, nil
}
```

**Token Scope Validation:**
```go
func (d *Doctor) validateTokenScopes(result *ValidationResult) {
    for i, token := range d.config.API.Auth.Tokens {
        for j, scopeStr := range token.Scopes {
            field := fmt.Sprintf("api.auth.tokens[%d].scopes[%d]", i, j)

            // Parse scope
            scope, err := parseScope(scopeStr)
            if err != nil {
                result.Errors = append(result.Errors, ValidationError{
                    Category: "token_scopes",
                    Message:  fmt.Sprintf("Token '%s' scope '%s': %v", token.Name, scopeStr, err),
                    Field:    field,
                })
                continue
            }

            // Validate manifest-driven scopes
            if scope.IsManifestDriven() {  // e.g., "withings:ro"
                plugin := d.pluginRegistry.Get(scope.Plugin)
                if plugin == nil {
                    result.Errors = append(result.Errors, ValidationError{
                        Category: "token_scopes",
                        Message:  fmt.Sprintf("Token '%s' scope '%s': plugin '%s' not found", token.Name, scopeStr, scope.Plugin),
                        Field:    field,
                    })
                    continue
                }

                // Validate type (ro/rw)
                if scope.Type != "ro" && scope.Type != "rw" && !strings.HasPrefix(scope.Type, "allow:") && !strings.HasPrefix(scope.Type, "deny:") {
                    result.Errors = append(result.Errors, ValidationError{
                        Category: "token_scopes",
                        Message:  fmt.Sprintf("Token '%s' scope '%s': invalid type (must be ro, rw, allow:cmd, or deny:cmd)", token.Name, scopeStr),
                        Field:    field,
                    })
                }

                // Validate specific command exists
                if strings.HasPrefix(scope.Type, "allow:") || strings.HasPrefix(scope.Type, "deny:") {
                    cmd := strings.TrimPrefix(strings.TrimPrefix(scope.Type, "allow:"), "deny:")
                    if !plugin.SupportsCommand(cmd) {
                        result.Errors = append(result.Errors, ValidationError{
                            Category: "token_scopes",
                            Message:  fmt.Sprintf("Token '%s' scope '%s': command '%s' not found in plugin '%s'", token.Name, scopeStr, cmd, scope.Plugin),
                            Field:    field,
                        })
                    }
                }
            }
        }
    }
}
```

**Plugin Reference Validation:**
```go
func (d *Doctor) validatePluginReferences(result *ValidationResult) {
    // Check plugins in scheduler config
    for name := range d.config.Plugins {
        if d.pluginRegistry.Get(name) == nil {
            result.Errors = append(result.Errors, ValidationError{
                Category: "plugin_refs",
                Message:  fmt.Sprintf("Plugin '%s' referenced in config but not found in plugins_dir", name),
                Field:    fmt.Sprintf("plugins.%s", name),
            })
        }
    }

    // Check plugins in routes
    for i, route := range d.config.Routes {
        if d.pluginRegistry.Get(route.From) == nil {
            result.Errors = append(result.Errors, ValidationError{
                Category: "plugin_refs",
                Message:  fmt.Sprintf("Route source plugin '%s' not found", route.From),
                Field:    fmt.Sprintf("routes[%d].from", i),
            })
        }
        if d.pluginRegistry.Get(route.To) == nil {
            result.Errors = append(result.Errors, ValidationError{
                Category: "plugin_refs",
                Message:  fmt.Sprintf("Route target plugin '%s' not found", route.To),
                Field:    fmt.Sprintf("routes[%d].to", i),
            })
        }
    }
}
```

**Webhook Path Validation:**
```go
func (d *Doctor) validateWebhooks(result *ValidationResult) {
    seen := make(map[string]int)
    for i, webhook := range d.config.Webhooks {
        // Normalize path (remove trailing slash for comparison)
        normalized := strings.TrimSuffix(webhook.Path, "/")

        if prevIdx, exists := seen[normalized]; exists {
            result.Errors = append(result.Errors, ValidationError{
                Category: "webhooks",
                Message:  fmt.Sprintf("Webhook path '%s' conflicts with webhooks[%d]", webhook.Path, prevIdx),
                Field:    fmt.Sprintf("webhooks[%d].path", i),
            })
        }
        seen[normalized] = i

        // Check plugin exists
        if d.pluginRegistry.Get(webhook.Plugin) == nil {
            result.Errors = append(result.Errors, ValidationError{
                Category: "webhooks",
                Message:  fmt.Sprintf("Webhook target plugin '%s' not found", webhook.Plugin),
                Field:    fmt.Sprintf("webhooks[%d].plugin", i),
            })
        }
    }
}
```

**Route Cycle Detection:**
```go
func (d *Doctor) validateRoutes(result *ValidationResult) {
    // Build adjacency list
    graph := make(map[string][]string)
    for _, route := range d.config.Routes {
        graph[route.From] = append(graph[route.From], route.To)
    }

    // Detect cycles via DFS
    visited := make(map[string]bool)
    recStack := make(map[string]bool)

    var hasCycle func(node string) bool
    hasCycle = func(node string) bool {
        visited[node] = true
        recStack[node] = true

        for _, neighbor := range graph[node] {
            if !visited[neighbor] && hasCycle(neighbor) {
                return true
            } else if recStack[neighbor] {
                return true
            }
        }

        recStack[node] = false
        return false
    }

    for node := range graph {
        if !visited[node] && hasCycle(node) {
            result.Errors = append(result.Errors, ValidationError{
                Category: "routes",
                Message:  fmt.Sprintf("Circular dependency detected in routes involving plugin '%s'", node),
                Field:    "routes",
            })
            break
        }
    }
}
```

**Environment Variable Warnings:**
```go
func (d *Doctor) warnMissingEnvVars(result *ValidationResult) {
    // Regex to find ${VAR} references
    envVarRegex := regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

    // Marshal config back to YAML to search for env vars
    configYAML, _ := yaml.Marshal(d.config)
    matches := envVarRegex.FindAllStringSubmatch(string(configYAML), -1)

    seen := make(map[string]bool)
    for _, match := range matches {
        varName := match[1]
        if !seen[varName] && os.Getenv(varName) == "" {
            result.Warnings = append(result.Warnings, ValidationWarning{
                Category: "env_vars",
                Message:  fmt.Sprintf("Environment variable ${%s} referenced but not set", varName),
                Field:    "",  // Hard to pinpoint exact field
            })
            seen[varName] = true
        }
    }
}
```

## CLI Integration

**cmd/ductile/main.go:**
```go
var doctorCmd = &cobra.Command{
    Use:   "doctor",
    Short: "Validate configuration file",
    RunE: func(cmd *cobra.Command, args []string) error {
        configPath, _ := cmd.Flags().GetString("config")
        strict, _ := cmd.Flags().GetBool("strict")
        format, _ := cmd.Flags().GetString("format")

        // Load config
        cfg, err := config.Load(configPath)
        if err != nil {
            return fmt.Errorf("failed to load config: %w", err)
        }

        // Discover plugins
        registry := plugin.NewRegistry()
        if err := registry.Discover(cfg.Service.PluginsDir); err != nil {
            return fmt.Errorf("failed to discover plugins: %w", err)
        }

        // Run validation
        doc := doctor.NewDoctor(cfg, registry, logger)
        result, err := doc.Validate(cmd.Context())
        if err != nil {
            return err
        }

        // Output results
        if format == "json" {
            printJSON(result)
        } else {
            printHuman(result)
        }

        // Exit code
        if !result.Valid {
            return cli.Exit("", 1)
        }
        if strict && len(result.Warnings) > 0 {
            return cli.Exit("", 2)
        }
        return nil
    },
}

func init() {
    doctorCmd.Flags().String("config", "config.yaml", "Path to config file")
    doctorCmd.Flags().Bool("strict", false, "Treat warnings as errors")
    doctorCmd.Flags().String("format", "human", "Output format (human, json)")
    rootCmd.AddCommand(doctorCmd)
}
```

## Testing

**Unit Tests:**
```go
func TestValidateTokenScopes(t *testing.T) {
    tests := []struct {
        name        string
        config      *config.Config
        plugins     []*plugin.Plugin
        expectError string
    }{
        {
            name: "valid manifest-driven scope",
            config: &config.Config{
                API: config.API{
                    Auth: config.Auth{
                        Tokens: []config.Token{
                            {Name: "test", Scopes: []string{"withings:ro"}},
                        },
                    },
                },
            },
            plugins: []*plugin.Plugin{
                {Name: "withings", Manifest: validManifest},
            },
            expectError: "",
        },
        {
            name: "invalid plugin reference",
            config: &config.Config{
                API: config.API{
                    Auth: config.Auth{
                        Tokens: []config.Token{
                            {Name: "test", Scopes: []string{"nonexistent:ro"}},
                        },
                    },
                },
            },
            plugins:     []*plugin.Plugin{},
            expectError: "plugin 'nonexistent' not found",
        },
    }
    // ...
}
```

**Integration Test:**
```bash
# Create invalid config
cat > /tmp/bad-config.yaml <<EOF
api:
  auth:
    tokens:
      - name: test
        scopes: ["nonexistent:ro"]
EOF

# Run doctor
./ductile doctor --config /tmp/bad-config.yaml
# Exit code: 1
# Output contains: "plugin 'nonexistent' not found"
```

## Use Cases

**1. Pre-deployment validation:**
```bash
# In CI/CD pipeline
ductile doctor --config production.yaml --strict || exit 1
```

**2. Config development:**
```bash
# While editing config.yaml
ductile doctor
# Fix errors
vim config.yaml
ductile doctor
```

**3. Debugging scope issues:**
```bash
# Check which scopes expand to which commands
ductile doctor | grep "expands to"
# external-cron: withings:ro (expands to: trigger:withings:poll)
```

## Dependencies

- Manifest command type metadata (#36) - For validating ro/rw scopes
- Token scopes implementation (#35) - For scope parsing logic
- Existing config loader (Sprint 1 ✓)
- Existing plugin discovery (Sprint 1 ✓)

## Follow-On Work

This enables:
- **Pre-commit hooks** - Run `ductile doctor --strict` before commits
- **Config testing in CI** - Catch invalid configs before deployment
- **TUI token tool (#38)** - Use validation logic to provide real-time feedback

## Narrative

The doctor command transforms "why isn't this working?" into "fix these 3 errors before starting." It's especially valuable when:
- Adding new tokens for webhook integrations (validates scopes before distributing)
- Refactoring routes (catches circular dependencies immediately)
- Updating manifests (validates existing token scopes still work)

Implementation is straightforward—most validation logic is pure functions over the config and plugin registry. The hardest part is providing helpful error messages with precise field paths, but JSON path notation (`api.auth.tokens[1].scopes[2]`) makes it tractable.

**Priority:** Normal. Nice to have before production, but not a blocker. Implement after #35 and #36 are stable.

- 2026-02-11: Implemented `ductile doctor` command in `internal/doctor` package. Validates service config, plugin references (including required config keys), token scopes (manifest-driven and low-level), webhook paths/secrets, route cycles, plus warnings for unused plugins, missing env vars, deprecated syntax, and suspicious schedules. Human and JSON output formats, --strict flag. 18 tests passing. (by @claude)
