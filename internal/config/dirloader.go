package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadDir loads configuration from a CONFIG_SPEC directory.
// Returns the compiled config and any integrity warnings.
func LoadDir(configDir string) (*Config, []string, error) {
	// 1. Discover files
	files, err := DiscoverConfigFiles(configDir)
	if err != nil {
		return nil, nil, fmt.Errorf("config discovery: %w", err)
	}

	// 2. Integrity verification
	intResult, err := VerifyIntegrity(configDir, files)
	if err != nil {
		return nil, nil, fmt.Errorf("integrity check: %w", err)
	}
	if !intResult.Passed {
		return nil, nil, fmt.Errorf("integrity verification failed:\n  %s\nRun 'senechal-gw config lock' to authorize the current state",
			joinLines(intResult.Errors))
	}

	// 3. Load root config.yaml
	cfg, err := loadAndParseFile(files.Config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config.yaml: %w", err)
	}
	cfg.ConfigDir = files.Root
	cfg.SourceFiles = make(map[string]*yaml.Node)

	// 4. Graft plugins/*.yaml
	if err := graftPlugins(cfg, files); err != nil {
		return nil, nil, err
	}

	// 5. Graft pipelines/*.yaml
	if err := graftPipelines(cfg, files); err != nil {
		return nil, nil, err
	}

	// 6. Load routes.yaml
	if files.Routes != "" {
		if err := graftRoutes(cfg, files.Routes); err != nil {
			return nil, nil, err
		}
	}

	// 7. Load webhooks.yaml
	if files.Webhooks != "" {
		if err := graftWebhooks(cfg, files.Webhooks); err != nil {
			return nil, nil, err
		}
	}

	// 8. Load tokens.yaml
	if files.Tokens != "" {
		if err := graftTokens(cfg, files.Tokens); err != nil {
			return nil, nil, err
		}
	}

	// 9. Apply defaults and validate
	cfg = applyConfigDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Apply plugin defaults
	for name, pluginConf := range cfg.Plugins {
		cfg.Plugins[name] = mergePluginDefaults(pluginConf)
	}

	return cfg, intResult.Warnings, nil
}

// loadAndParseFile reads a YAML file, interpolates env vars, and parses it.
func loadAndParseFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	interpolated := interpolateEnv(string(data))
	var cfg Config
	if err := yaml.Unmarshal([]byte(interpolated), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return &cfg, nil
}

// graftPlugins merges plugin definitions from plugins/*.yaml into cfg.
func graftPlugins(cfg *Config, files *ConfigFiles) error {
	if cfg.Plugins == nil {
		cfg.Plugins = make(map[string]PluginConf)
	}
	for _, path := range files.Plugins {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}
		interpolated := interpolateEnv(string(data))

		var pf PluginsFileConfig
		if err := yaml.Unmarshal([]byte(interpolated), &pf); err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}
		for name, plugin := range pf.Plugins {
			cfg.Plugins[name] = plugin
		}
	}
	return nil
}

// graftPipelines appends pipeline entries from pipelines/*.yaml into cfg.
func graftPipelines(cfg *Config, files *ConfigFiles) error {
	for _, path := range files.Pipelines {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}
		interpolated := interpolateEnv(string(data))

		var pf PipelinesFileConfig
		if err := yaml.Unmarshal([]byte(interpolated), &pf); err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}
		cfg.Pipelines = append(cfg.Pipelines, pf.Pipelines...)
	}
	return nil
}

// graftRoutes appends routes from routes.yaml into cfg.
func graftRoutes(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read routes.yaml: %w", err)
	}
	interpolated := interpolateEnv(string(data))

	var rf RoutesFileConfig
	if err := yaml.Unmarshal([]byte(interpolated), &rf); err != nil {
		return fmt.Errorf("failed to parse routes.yaml: %w", err)
	}
	cfg.Routes = append(cfg.Routes, rf.Routes...)
	return nil
}

// graftWebhooks loads webhooks from webhooks.yaml into cfg.
func graftWebhooks(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read webhooks.yaml: %w", err)
	}
	interpolated := interpolateEnv(string(data))

	var wf WebhooksFileConfig
	if err := yaml.Unmarshal([]byte(interpolated), &wf); err != nil {
		return fmt.Errorf("failed to parse webhooks.yaml: %w", err)
	}

	if cfg.Webhooks == nil {
		cfg.Webhooks = &WebhooksConfig{}
	}
	cfg.Webhooks.Endpoints = append(cfg.Webhooks.Endpoints, wf.Webhooks...)
	return nil
}

// graftTokens loads token entries from tokens.yaml into cfg.
func graftTokens(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read tokens.yaml: %w", err)
	}
	interpolated := interpolateEnv(string(data))

	var tf TokensFileConfig
	if err := yaml.Unmarshal([]byte(interpolated), &tf); err != nil {
		return fmt.Errorf("failed to parse tokens.yaml: %w", err)
	}
	cfg.Tokens = append(cfg.Tokens, tf.Tokens...)
	return nil
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n  "
		}
		result += line
	}
	return result
}
