package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// graftTokens loads token entries from tokens.yaml into cfg.
func graftTokens(cfg *Config, path string) error {
	// #nosec G304 -- config paths are operator-controlled local inputs.
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
