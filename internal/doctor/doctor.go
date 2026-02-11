// Package doctor validates senechal-gw configuration and plugin setup.
package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/mattjoyce/senechal-gw/internal/config"
	"github.com/mattjoyce/senechal-gw/internal/plugin"
)

// Result holds the outcome of a validation run.
type Result struct {
	Valid    bool      `json:"valid"`
	Errors   []Issue   `json:"errors,omitempty"`
	Warnings []Issue   `json:"warnings,omitempty"`
}

// Issue describes a single validation error or warning.
type Issue struct {
	Category string `json:"category"`
	Message  string `json:"message"`
	Field    string `json:"field,omitempty"`
}

// Doctor validates configuration against discovered plugins.
type Doctor struct {
	cfg      *config.Config
	registry *plugin.Registry
}

// New creates a Doctor from a loaded config and plugin registry.
func New(cfg *config.Config, registry *plugin.Registry) *Doctor {
	return &Doctor{cfg: cfg, registry: registry}
}

// Validate runs all checks and returns a result.
func (d *Doctor) Validate() *Result {
	r := &Result{Valid: true}

	d.validateServiceConfig(r)
	d.validatePluginRefs(r)
	d.validateAPIConfig(r)
	d.validateTokenScopes(r)
	d.validateWebhooks(r)
	d.validateRoutes(r)
	d.warnUnusedPlugins(r)
	d.warnMissingEnvVars(r)
	d.warnDeprecatedSyntax(r)
	d.warnSuspiciousSchedule(r)

	r.Valid = len(r.Errors) == 0
	return r
}

func (d *Doctor) addError(r *Result, category, field, msg string) {
	r.Errors = append(r.Errors, Issue{Category: category, Field: field, Message: msg})
}

func (d *Doctor) addWarning(r *Result, category, field, msg string) {
	r.Warnings = append(r.Warnings, Issue{Category: category, Field: field, Message: msg})
}

// validateServiceConfig checks required service fields.
func (d *Doctor) validateServiceConfig(r *Result) {
	if d.cfg.PluginsDir == "" {
		d.addError(r, "service", "plugins_dir", "plugins_dir is required")
	}
	if d.cfg.State.Path == "" {
		d.addError(r, "service", "state.path", "state.path is required")
	}
	if d.cfg.Service.TickInterval <= 0 {
		d.addError(r, "service", "service.tick_interval", "tick_interval must be positive")
	}
}

// validatePluginRefs checks that plugins in config are discoverable.
func (d *Doctor) validatePluginRefs(r *Result) {
	for name, pc := range d.cfg.Plugins {
		if !pc.Enabled {
			continue
		}
		if _, ok := d.registry.Get(name); !ok {
			d.addError(r, "plugin_refs", fmt.Sprintf("plugins.%s", name),
				fmt.Sprintf("plugin %q in config but not found in plugins_dir", name))
		}

		// Check required config keys
		if p, ok := d.registry.Get(name); ok && p.ConfigKeys != nil {
			for _, key := range p.ConfigKeys.Required {
				if pc.Config == nil {
					d.addError(r, "plugin_refs", fmt.Sprintf("plugins.%s.config", name),
						fmt.Sprintf("plugin %q requires config key %q", name, key))
					continue
				}
				if _, exists := pc.Config[key]; !exists {
					d.addError(r, "plugin_refs", fmt.Sprintf("plugins.%s.config.%s", name, key),
						fmt.Sprintf("plugin %q requires config key %q", name, key))
				}
			}
		}
	}
}

// validateAPIConfig checks API server settings.
func (d *Doctor) validateAPIConfig(r *Result) {
	if !d.cfg.API.Enabled {
		return
	}
	if d.cfg.API.Listen == "" {
		d.addError(r, "api", "api.listen", "api.listen is required when API is enabled")
	}
	if d.cfg.API.Auth.APIKey == "" && len(d.cfg.API.Auth.Tokens) == 0 {
		d.addWarning(r, "api", "api.auth", "API enabled but no authentication configured")
	}
}

// validateTokenScopes checks that scope references resolve to real plugins/commands.
func (d *Doctor) validateTokenScopes(r *Result) {
	for i, token := range d.cfg.API.Auth.Tokens {
		for j, scope := range token.Scopes {
			field := fmt.Sprintf("api.auth.tokens[%d].scopes[%d]", i, j)
			d.validateSingleScope(r, scope, field)
		}
	}
}

func (d *Doctor) validateSingleScope(r *Result, scope, field string) {
	// Admin wildcard
	if scope == "*" {
		return
	}

	parts := strings.SplitN(scope, ":", 2)
	if len(parts) < 2 {
		d.addError(r, "token_scopes", field,
			fmt.Sprintf("invalid scope %q (expected format: resource:access or action:resource:command)", scope))
		return
	}

	first, second := parts[0], parts[1]

	// Low-level: action:resource or action:resource:command
	if first == "read" || first == "trigger" || first == "admin" {
		// Valid low-level scope syntax
		return
	}

	// Manifest-driven: plugin:ro, plugin:rw, plugin:allow:cmd, plugin:deny:cmd
	pluginName := first
	p, ok := d.registry.Get(pluginName)
	if !ok {
		// Check if it's a known non-plugin resource
		if pluginName == "jobs" || pluginName == "events" || pluginName == "healthz" || pluginName == "queue" {
			return
		}
		d.addError(r, "token_scopes", field,
			fmt.Sprintf("scope %q references unknown plugin %q", scope, pluginName))
		return
	}

	switch {
	case second == "ro" || second == "rw":
		// Valid
	case strings.HasPrefix(second, "allow:"):
		cmd := strings.TrimPrefix(second, "allow:")
		if cmd != "*" && !p.SupportsCommand(cmd) {
			d.addError(r, "token_scopes", field,
				fmt.Sprintf("scope %q: plugin %q has no command %q", scope, pluginName, cmd))
		}
	case strings.HasPrefix(second, "deny:"):
		cmd := strings.TrimPrefix(second, "deny:")
		if cmd != "*" && !p.SupportsCommand(cmd) {
			d.addWarning(r, "token_scopes", field,
				fmt.Sprintf("scope %q: plugin %q has no command %q (deny is a no-op)", scope, pluginName, cmd))
		}
	default:
		d.addError(r, "token_scopes", field,
			fmt.Sprintf("scope %q: invalid access type %q (expected ro, rw, allow:cmd, or deny:cmd)", scope, second))
	}
}

// validateWebhooks checks for path conflicts and plugin references.
func (d *Doctor) validateWebhooks(r *Result) {
	if d.cfg.Webhooks == nil {
		return
	}

	seen := make(map[string]int)
	for i, ep := range d.cfg.Webhooks.Endpoints {
		field := fmt.Sprintf("webhooks.endpoints[%d]", i)

		// Check plugin exists
		if _, ok := d.registry.Get(ep.Plugin); !ok {
			d.addError(r, "webhooks", field+".plugin",
				fmt.Sprintf("webhook %q targets plugin %q which was not discovered", ep.Path, ep.Plugin))
		}

		// Check for path conflicts
		normalized := strings.TrimSuffix(ep.Path, "/")
		if prevIdx, exists := seen[normalized]; exists {
			d.addError(r, "webhooks", field+".path",
				fmt.Sprintf("webhook path %q conflicts with webhooks.endpoints[%d]", ep.Path, prevIdx))
		}
		seen[normalized] = i

		// Check secret configured
		if ep.Secret == "" && ep.SecretRef == "" {
			d.addError(r, "webhooks", field,
				fmt.Sprintf("webhook %q: either secret or secret_ref is required", ep.Path))
		}
	}
}

// validateRoutes checks plugin refs and circular dependencies.
func (d *Doctor) validateRoutes(r *Result) {
	if len(d.cfg.Routes) == 0 {
		return
	}

	graph := make(map[string][]string)
	for i, route := range d.cfg.Routes {
		field := fmt.Sprintf("routes[%d]", i)

		if _, ok := d.registry.Get(route.From); !ok {
			if _, inConfig := d.cfg.Plugins[route.From]; !inConfig {
				d.addError(r, "routes", field+".from",
					fmt.Sprintf("route source plugin %q not found", route.From))
			}
		}
		if _, ok := d.registry.Get(route.To); !ok {
			if _, inConfig := d.cfg.Plugins[route.To]; !inConfig {
				d.addError(r, "routes", field+".to",
					fmt.Sprintf("route target plugin %q not found", route.To))
			}
		}

		graph[route.From] = append(graph[route.From], route.To)
	}

	// Cycle detection via DFS
	visited := make(map[string]int) // 0=unvisited, 1=in-stack, 2=done
	var hasCycle func(node string) bool
	hasCycle = func(node string) bool {
		visited[node] = 1
		for _, next := range graph[node] {
			if visited[next] == 1 {
				return true
			}
			if visited[next] == 0 && hasCycle(next) {
				return true
			}
		}
		visited[node] = 2
		return false
	}

	for node := range graph {
		if visited[node] == 0 && hasCycle(node) {
			d.addError(r, "routes", "routes",
				fmt.Sprintf("circular dependency detected involving plugin %q", node))
			break
		}
	}
}

// warnUnusedPlugins warns about discovered plugins not referenced in config.
func (d *Doctor) warnUnusedPlugins(r *Result) {
	for name := range d.registry.All() {
		if _, inConfig := d.cfg.Plugins[name]; !inConfig {
			d.addWarning(r, "unused", "",
				fmt.Sprintf("plugin %q discovered but not referenced in config", name))
		}
	}
}

// warnMissingEnvVars warns about ${VAR} references where VAR is not set.
func (d *Doctor) warnMissingEnvVars(r *Result) {
	envVarRe := regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

	// Check token values and webhook secrets for unresolved env vars
	for i, token := range d.cfg.API.Auth.Tokens {
		if token.Token == "" {
			d.addWarning(r, "env_vars", fmt.Sprintf("api.auth.tokens[%d].token", i),
				"token value is empty (possibly unresolved environment variable)")
		}
	}

	// Check webhook secrets
	if d.cfg.Webhooks != nil {
		for i, ep := range d.cfg.Webhooks.Endpoints {
			if ep.Secret != "" && envVarRe.MatchString(ep.Secret) {
				for _, m := range envVarRe.FindAllStringSubmatch(ep.Secret, -1) {
					if os.Getenv(m[1]) == "" {
						d.addWarning(r, "env_vars", fmt.Sprintf("webhooks.endpoints[%d].secret", i),
							fmt.Sprintf("environment variable ${%s} not set", m[1]))
					}
				}
			}
		}
	}
}

// warnDeprecatedSyntax warns about legacy config patterns.
func (d *Doctor) warnDeprecatedSyntax(r *Result) {
	if d.cfg.API.Auth.APIKey != "" && len(d.cfg.API.Auth.Tokens) > 0 {
		d.addWarning(r, "deprecated", "api.auth",
			"both api_key and tokens configured; prefer tokens array only")
	}
	if d.cfg.API.Auth.APIKey != "" && len(d.cfg.API.Auth.Tokens) == 0 {
		d.addWarning(r, "deprecated", "api.auth.api_key",
			"legacy api_key grants full access; migrate to tokens array with scopes")
	}
}

// warnSuspiciousSchedule warns about intervals that seem too short or too long.
func (d *Doctor) warnSuspiciousSchedule(r *Result) {
	for name, pc := range d.cfg.Plugins {
		if pc.Schedule == nil || !pc.Enabled {
			continue
		}
		interval, err := config.ParseInterval(pc.Schedule.Every)
		if err != nil {
			d.addError(r, "schedule", fmt.Sprintf("plugins.%s.schedule.every", name),
				fmt.Sprintf("invalid schedule interval %q: %v", pc.Schedule.Every, err))
			continue
		}
		if interval.Minutes() < 1 {
			d.addWarning(r, "schedule", fmt.Sprintf("plugins.%s.schedule.every", name),
				fmt.Sprintf("schedule interval %q is very short (< 1m)", pc.Schedule.Every))
		}
		if interval.Hours() > 24 {
			// weekly/monthly are fine, just flag unusual custom values
		}
	}
}

// FormatHuman returns a human-readable validation report.
func FormatHuman(r *Result) string {
	var b strings.Builder

	if r.Valid && len(r.Warnings) == 0 {
		b.WriteString("Configuration valid.\n")
		return b.String()
	}

	if r.Valid && len(r.Warnings) > 0 {
		b.WriteString("Configuration valid")
		fmt.Fprintf(&b, " (%d warning(s))\n", len(r.Warnings))
	}

	if !r.Valid {
		fmt.Fprintf(&b, "Configuration invalid (%d error(s), %d warning(s))\n", len(r.Errors), len(r.Warnings))
	}

	for _, e := range r.Errors {
		if e.Field != "" {
			fmt.Fprintf(&b, "  ERROR [%s] %s: %s\n", e.Category, e.Field, e.Message)
		} else {
			fmt.Fprintf(&b, "  ERROR [%s] %s\n", e.Category, e.Message)
		}
	}
	for _, w := range r.Warnings {
		if w.Field != "" {
			fmt.Fprintf(&b, "  WARN  [%s] %s: %s\n", w.Category, w.Field, w.Message)
		} else {
			fmt.Fprintf(&b, "  WARN  [%s] %s\n", w.Category, w.Message)
		}
	}

	return b.String()
}

// FormatJSON returns the result as indented JSON.
func FormatJSON(r *Result) (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
