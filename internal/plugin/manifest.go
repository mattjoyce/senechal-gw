package plugin

// Manifest defines the structure of a plugin's manifest.yaml file.
type Manifest struct {
	Name        string       `yaml:"name"`
	Version     string       `yaml:"version"`
	Protocol    int          `yaml:"protocol"`
	Entrypoint  string       `yaml:"entrypoint"`
	Description string       `yaml:"description,omitempty"`
	Commands    []string     `yaml:"commands"`
	ConfigKeys  *ConfigKeys  `yaml:"config_keys,omitempty"`
}

// ConfigKeys defines required and optional configuration keys for a plugin.
type ConfigKeys struct {
	Required []string `yaml:"required,omitempty"`
	Optional []string `yaml:"optional,omitempty"`
}

// Plugin represents a discovered and validated plugin.
type Plugin struct {
	Name        string   // Plugin name from manifest
	Path        string   // Absolute path to plugin directory
	Entrypoint  string   // Absolute path to entrypoint executable
	Protocol    int      // Protocol version
	Version     string   // Plugin version
	Description string   // Human-readable description
	Commands    []string // Supported commands (poll, handle, health, init)
	ConfigKeys  *ConfigKeys
}

// SupportsCommand checks if the plugin supports a given command.
func (p *Plugin) SupportsCommand(cmd string) bool {
	for _, c := range p.Commands {
		if c == cmd {
			return true
		}
	}
	return false
}
