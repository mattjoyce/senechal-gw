package config

import (
	"fmt"
	"strings"
)

// ConfigValidator validates cross-file references in multi-file config mode.
type ConfigValidator struct {
	config *Config
	tokens map[string]string
}

// ValidateCrossReferences checks that all cross-file references are valid.
func (v *ConfigValidator) ValidateCrossReferences() error {
	// Validate routes reference valid plugins
	if err := v.validateRoutes(); err != nil {
		return err
	}

	// Validate webhooks reference valid plugins and tokens
	if err := v.validateWebhooks(); err != nil {
		return err
	}

	// Validate plugin configs with _ref suffixes reference valid tokens
	if err := v.validatePluginTokenRefs(); err != nil {
		return err
	}

	return nil
}

// validateRoutes checks that route from/to fields reference enabled plugins.
func (v *ConfigValidator) validateRoutes() error {
	if len(v.config.Routes) == 0 {
		return nil
	}

	for i, route := range v.config.Routes {
		// Validate 'from' plugin exists
		if _, exists := v.config.Plugins[route.From]; !exists {
			return fmt.Errorf("route[%d]: 'from' plugin %q does not exist", i, route.From)
		}

		// Validate 'to' plugin exists
		if _, exists := v.config.Plugins[route.To]; !exists {
			return fmt.Errorf("route[%d]: 'to' plugin %q does not exist", i, route.To)
		}

		// Check if plugins are enabled (warning only, not fatal)
		if fromPlugin := v.config.Plugins[route.From]; !fromPlugin.Enabled {
			// Non-fatal: just log warning (actual logging happens at runtime)
		}
		if toPlugin := v.config.Plugins[route.To]; !toPlugin.Enabled {
			// Non-fatal: just log warning (actual logging happens at runtime)
		}
	}

	return nil
}

// validateWebhooks checks that webhook endpoints reference valid plugins and secrets.
func (v *ConfigValidator) validateWebhooks() error {
	if v.config.Webhooks == nil {
		return nil
	}

	for i, endpoint := range v.config.Webhooks.Endpoints {
		// Validate plugin exists
		if _, exists := v.config.Plugins[endpoint.Plugin]; !exists {
			return fmt.Errorf("webhook[%d] (%s): plugin %q does not exist",
				i, endpoint.Path, endpoint.Plugin)
		}

		// Validate secret or secret_ref is provided
		if endpoint.Secret == "" && endpoint.SecretRef == "" {
			return fmt.Errorf("webhook[%d] (%s): either 'secret' or 'secret_ref' is required",
				i, endpoint.Path)
		}

		// If using secret_ref, validate it exists in tokens
		if endpoint.SecretRef != "" {
			if _, exists := v.tokens[endpoint.SecretRef]; !exists {
				return fmt.Errorf("webhook[%d] (%s): secret_ref %q not found in tokens.yaml",
					i, endpoint.Path, endpoint.SecretRef)
			}
		}

		// Validate required fields
		if endpoint.SignatureHeader == "" {
			return fmt.Errorf("webhook[%d] (%s): signature_header is required",
				i, endpoint.Path)
		}
	}

	return nil
}

// validatePluginTokenRefs checks plugin config fields ending with _ref reference valid tokens.
func (v *ConfigValidator) validatePluginTokenRefs() error {
	for pluginName, plugin := range v.config.Plugins {
		if plugin.Config == nil {
			continue
		}

		for key, value := range plugin.Config {
			// Check if key ends with _ref
			if strings.HasSuffix(key, "_ref") {
				strValue, ok := value.(string)
				if !ok {
					return fmt.Errorf("plugin %q: config field %q must be a string",
						pluginName, key)
				}

				// Validate token exists
				if _, exists := v.tokens[strValue]; !exists {
					return fmt.Errorf("plugin %q: config field %q references token %q not found in tokens.yaml",
						pluginName, key, strValue)
				}
			}
		}
	}

	return nil
}
