package webhook

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mattjoyce/senechal-gw/internal/config"
)

// FromGlobalConfig converts config.WebhooksConfig to webhook.Config.
// Resolves secret references and parses max body sizes.
func FromGlobalConfig(wc *config.WebhooksConfig, tokens map[string]string) (Config, error) {
	if wc == nil {
		return Config{}, fmt.Errorf("webhooks config is nil")
	}

	cfg := Config{
		Listen:    wc.Listen,
		Endpoints: make([]EndpointConfig, len(wc.Endpoints)),
	}

	for i, ep := range wc.Endpoints {
		// Resolve secret (SecretRef takes precedence over Secret)
		secret := ep.Secret
		if ep.SecretRef != "" {
			resolvedSecret, ok := tokens[ep.SecretRef]
			if !ok {
				return Config{}, fmt.Errorf("webhook endpoint %q: secret_ref %q not found in tokens", ep.Path, ep.SecretRef)
			}
			secret = resolvedSecret
		}

		if secret == "" {
			return Config{}, fmt.Errorf("webhook endpoint %q: no secret or secret_ref configured", ep.Path)
		}

		// Parse max body size (e.g., "1MB", "2048576")
		maxBodySize, err := parseMaxBodySize(ep.MaxBodySize)
		if err != nil {
			return Config{}, fmt.Errorf("webhook endpoint %q: invalid max_body_size %q: %w", ep.Path, ep.MaxBodySize, err)
		}

		cfg.Endpoints[i] = EndpointConfig{
			Path:            ep.Path,
			Plugin:          ep.Plugin,
			Command:         DefaultCommand, // Can be extended in config if needed
			Secret:          secret,
			SignatureHeader: ep.SignatureHeader,
			MaxBodySize:     maxBodySize,
		}
	}

	return cfg, nil
}

// parseMaxBodySize parses size strings like "1MB", "2048576", "1048576" to bytes.
// Returns DefaultMaxBodySize if empty.
func parseMaxBodySize(size string) (int64, error) {
	if size == "" {
		return DefaultMaxBodySize, nil
	}

	// Handle unit suffixes (KB, MB, GB)
	upper := strings.ToUpper(size)
	multiplier := int64(1)

	if strings.HasSuffix(upper, "KB") {
		multiplier = 1024
		size = strings.TrimSuffix(upper, "KB")
	} else if strings.HasSuffix(upper, "MB") {
		multiplier = 1024 * 1024
		size = strings.TrimSuffix(upper, "MB")
	} else if strings.HasSuffix(upper, "GB") {
		multiplier = 1024 * 1024 * 1024
		size = strings.TrimSuffix(upper, "GB")
	}

	// Parse numeric value
	value, err := strconv.ParseInt(strings.TrimSpace(size), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size value: %w", err)
	}

	if value <= 0 {
		return 0, fmt.Errorf("size must be positive")
	}

	result := value * multiplier
	if result < 0 { // Check for overflow
		return 0, fmt.Errorf("size too large")
	}

	return result, nil
}
