package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Load reads and parses a config file, applying environment variable interpolation.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	// Apply environment variable interpolation
	interpolated := interpolateEnv(string(data))

	// Parse YAML
	cfg := Defaults()
	if err := yaml.Unmarshal([]byte(interpolated), cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Validate
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Apply plugin defaults
	for name, pluginConf := range cfg.Plugins {
		merged := mergePluginDefaults(pluginConf)
		cfg.Plugins[name] = merged
	}

	return cfg, nil
}

// interpolateEnv replaces ${VAR} with environment variable values.
// Undefined variables are left as-is (not expanded).
func interpolateEnv(input string) string {
	return envVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		// Extract variable name from ${VAR}
		varName := envVarPattern.FindStringSubmatch(match)[1]

		// Look up environment variable
		if value, exists := os.LookupEnv(varName); exists {
			return value
		}

		// If not found, leave the placeholder (will fail validation if required)
		return match
	})
}

// validate performs basic validation on the configuration.
func validate(cfg *Config) error {
	// Service validation
	if cfg.Service.TickInterval <= 0 {
		return fmt.Errorf("service.tick_interval must be positive")
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[cfg.Service.LogLevel] {
		return fmt.Errorf("service.log_level must be one of: debug, info, warn, error (got %q)", cfg.Service.LogLevel)
	}

	// State validation
	if cfg.State.Path == "" {
		return fmt.Errorf("state.path is required")
	}

	// Plugins dir validation
	if cfg.PluginsDir == "" {
		return fmt.Errorf("plugins_dir is required")
	}

	// API auth validation
	if cfg.API.Enabled {
		// If tokens are configured, validate them. api_key remains supported for back-compat.
		if envVarPattern.MatchString(cfg.API.Auth.APIKey) {
			matches := envVarPattern.FindStringSubmatch(cfg.API.Auth.APIKey)
			if len(matches) > 1 {
				return fmt.Errorf("api.auth.api_key: environment variable ${%s} is not set", matches[1])
			}
			return fmt.Errorf("api.auth.api_key: unresolved environment variable")
		}
		for i, tok := range cfg.API.Auth.Tokens {
			if tok.Token == "" {
				return fmt.Errorf("api.auth.tokens[%d].token is required", i)
			}
			if envVarPattern.MatchString(tok.Token) {
				matches := envVarPattern.FindStringSubmatch(tok.Token)
				if len(matches) > 1 {
					return fmt.Errorf("api.auth.tokens[%d].token: environment variable ${%s} is not set", i, matches[1])
				}
				return fmt.Errorf("api.auth.tokens[%d].token: unresolved environment variable", i)
			}
			if len(tok.Scopes) == 0 {
				return fmt.Errorf("api.auth.tokens[%d].scopes must be non-empty", i)
			}
		}
	}

	// Plugin validation
	for name, plugin := range cfg.Plugins {
		if !plugin.Enabled {
			continue // Skip disabled plugins
		}

		// Schedule required for enabled plugins
		if plugin.Schedule == nil {
			return fmt.Errorf("plugin %q: schedule is required for enabled plugins", name)
		}

		if plugin.Schedule.Every == "" {
			return fmt.Errorf("plugin %q: schedule.every is required", name)
		}

		// Validate schedule.every values (MVP subset)
		validIntervals := []string{"5m", "15m", "30m", "hourly", "2h", "6h", "daily", "weekly", "monthly"}
		valid := false
		for _, interval := range validIntervals {
			if plugin.Schedule.Every == interval {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("plugin %q: schedule.every must be one of %v (got %q)",
				name, validIntervals, plugin.Schedule.Every)
		}

		// Check for unresolved env vars in config (security: no secrets leaked in logs)
		if plugin.Config != nil {
			if err := checkUnresolvedEnvVars(plugin.Config, name); err != nil {
				return err
			}
		}
	}

	return nil
}

// checkUnresolvedEnvVars recursively checks for ${VAR} placeholders in config values.
func checkUnresolvedEnvVars(data map[string]interface{}, pluginName string) error {
	for key, value := range data {
		switch v := value.(type) {
		case string:
			if envVarPattern.MatchString(v) {
				// Extract variable name for better error message
				matches := envVarPattern.FindStringSubmatch(v)
				if len(matches) > 1 {
					return fmt.Errorf("plugin %q: environment variable ${%s} is not set", pluginName, matches[1])
				}
				return fmt.Errorf("plugin %q: unresolved environment variable in config.%s", pluginName, key)
			}
		case map[string]interface{}:
			if err := checkUnresolvedEnvVars(v, pluginName); err != nil {
				return err
			}
		}
	}
	return nil
}

// mergePluginDefaults applies default values to plugin config where not specified.
func mergePluginDefaults(plugin PluginConf) PluginConf {
	defaults := DefaultPluginConf()

	if plugin.Retry == nil {
		plugin.Retry = defaults.Retry
	}

	if plugin.Timeouts == nil {
		plugin.Timeouts = defaults.Timeouts
	}

	if plugin.CircuitBreaker == nil {
		plugin.CircuitBreaker = defaults.CircuitBreaker
	}

	if plugin.MaxOutstandingPolls == 0 {
		plugin.MaxOutstandingPolls = defaults.MaxOutstandingPolls
	}

	return plugin
}

// ParseInterval converts schedule interval strings to durations.
// Returns 0 for special cases like "daily", "weekly", "monthly" (handled by scheduler).
func ParseInterval(interval string) (time.Duration, error) {
	// Direct duration strings
	switch interval {
	case "hourly":
		return 1 * time.Hour, nil
	case "daily", "weekly", "monthly":
		return 0, nil // Special handling in scheduler
	}

	// Try parsing as duration (e.g., "5m", "2h")
	d, err := time.ParseDuration(interval)
	if err != nil {
		return 0, fmt.Errorf("invalid schedule interval %q: %w", interval, err)
	}

	if d <= 0 {
		return 0, fmt.Errorf("schedule interval must be positive: %q", interval)
	}

	return d, nil
}
