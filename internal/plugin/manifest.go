package plugin

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// CommandType is a coarse permission hint for a command.
// It is used by authorization middleware to separate read-only commands from
// commands that may mutate state or cause side effects.
type CommandType string

const (
	CommandTypeRead  CommandType = "read"
	CommandTypeWrite CommandType = "write"
)

func (t CommandType) valid() bool {
	return t == CommandTypeRead || t == CommandTypeWrite
}

// Command declares a supported plugin command and its type.
type Command struct {
	Name         string      `yaml:"name"`
	Type         CommandType `yaml:"type"`
	Description  string      `yaml:"description,omitempty"`
	InputSchema  any         `yaml:"input_schema,omitempty"`
	OutputSchema any         `yaml:"output_schema,omitempty"`
}

// GetFullInputSchema returns the expanded JSON Schema for the input.
func (c Command) GetFullInputSchema() any {
	return expandSchema(c.InputSchema)
}

// GetFullOutputSchema returns the expanded JSON Schema for the output.
func (c Command) GetFullOutputSchema() any {
	return expandSchema(c.OutputSchema)
}

func expandSchema(schema any) any {
	if schema == nil {
		return nil
	}

	m, ok := schema.(map[string]any)
	if !ok {
		return schema
	}

	// If it already looks like a JSON schema (has "type"), return as-is
	if _, hasType := m["type"]; hasType {
		return schema
	}

	// Otherwise, treat as a compact map of property:type
	properties := make(map[string]any)
	for k, v := range m {
		propType, isString := v.(string)
		if isString {
			properties[k] = map[string]string{"type": propType}
		} else {
			properties[k] = v
		}
	}

	return map[string]any{
		"type":       "object",
		"properties": properties,
	}
}

// Commands is a list of supported commands.
//
// Backward-compatible formats:
//   - legacy string array: commands: [poll, health]
//   - object array: commands: [{name: poll, type: write}, {name: health, type: read}]
type Commands []Command

func defaultCommandType(name string) CommandType {
	// Conservative default for legacy manifests: only "health" is treated as read.
	// Everything else is considered write until explicitly annotated.
	if name == "health" {
		return CommandTypeRead
	}
	return CommandTypeWrite
}

func (c *Commands) UnmarshalYAML(n *yaml.Node) error {
	if n == nil {
		*c = nil
		return nil
	}
	if n.Kind != yaml.SequenceNode {
		return fmt.Errorf("commands must be a sequence")
	}

	out := make([]Command, 0, len(n.Content))
	for _, item := range n.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			name := strings.TrimSpace(item.Value)
			out = append(out, Command{
				Name: name,
				Type: defaultCommandType(name),
			})
		case yaml.MappingNode:
			var tmp Command
			if err := item.Decode(&tmp); err != nil {
				return fmt.Errorf("invalid command object: %w", err)
			}
			tmp.Name = strings.TrimSpace(tmp.Name)
			if tmp.Type == "" {
				tmp.Type = defaultCommandType(tmp.Name)
			}
			out = append(out, tmp)
		default:
			return fmt.Errorf("invalid command entry (must be string or object)")
		}
	}

	*c = out
	return nil
}

// Manifest defines the structure of a plugin's manifest.yaml file.
type Manifest struct {
	Name        string      `yaml:"name"`
	Version     string      `yaml:"version"`
	Protocol    int         `yaml:"protocol"`
	Entrypoint  string      `yaml:"entrypoint"`
	Description string      `yaml:"description,omitempty"`
	Commands    Commands    `yaml:"commands"`
	ConfigKeys  *ConfigKeys `yaml:"config_keys,omitempty"`
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
	Commands    Commands // Supported commands (poll, handle, health, init)
	ConfigKeys  *ConfigKeys
}

// SupportsCommand checks if the plugin supports a given command.
func (p *Plugin) SupportsCommand(cmd string) bool {
	for _, c := range p.Commands {
		if c.Name == cmd {
			return true
		}
	}
	return false
}

// CommandTypeFor returns the declared type for a command.
func (p *Plugin) CommandTypeFor(cmd string) (CommandType, bool) {
	for _, c := range p.Commands {
		if c.Name == cmd {
			return c.Type, true
		}
	}
	return "", false
}

// GetReadCommands returns the command names marked type=read, preserving manifest order.
func (p *Plugin) GetReadCommands() []string {
	var out []string
	for _, c := range p.Commands {
		if c.Type == CommandTypeRead {
			out = append(out, c.Name)
		}
	}
	return out
}

// GetWriteCommands returns the command names marked type=write, preserving manifest order.
func (p *Plugin) GetWriteCommands() []string {
	var out []string
	for _, c := range p.Commands {
		if c.Type == CommandTypeWrite {
			out = append(out, c.Name)
		}
	}
	return out
}
