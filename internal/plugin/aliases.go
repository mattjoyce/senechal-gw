package plugin

import (
	"fmt"
	"strings"

	"github.com/mattjoyce/ductile/internal/config"
)

// Alias describes a config-level plugin instance mapping to a base manifest.
type Alias struct {
	Name string
	Uses string
}

// ApplyAliases registers config-level plugin instances that reuse a base manifest.
// It mutates the provided registry by adding cloned entries for each alias.
func ApplyAliases(registry *Registry, plugins map[string]config.PluginConf) ([]Alias, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry is required")
	}
	if len(plugins) == 0 {
		return nil, nil
	}

	aliases := make([]Alias, 0)
	for name, conf := range plugins {
		uses := strings.TrimSpace(conf.Uses)
		if uses == "" {
			continue
		}
		if name == uses {
			return nil, fmt.Errorf("plugin %q: uses must reference a different base plugin", name)
		}
		base, ok := registry.Get(uses)
		if !ok {
			return nil, fmt.Errorf("plugin %q: uses target %q was not discovered", name, uses)
		}
		if _, exists := registry.Get(name); exists {
			return nil, fmt.Errorf("plugin %q: alias name conflicts with existing plugin", name)
		}
		clone := *base
		clone.Name = name
		if err := registry.Add(&clone); err != nil {
			return nil, err
		}
		aliases = append(aliases, Alias{Name: name, Uses: uses})
	}

	return aliases, nil
}
