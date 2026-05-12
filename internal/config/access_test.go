package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var assert testAssert

type testAssert struct{}

func (testAssert) NoError(t *testing.T, err error, _ ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func (testAssert) Error(t *testing.T, err error, _ ...any) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
}

func (testAssert) Equal(t *testing.T, want, got any, _ ...any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func (testAssert) False(t *testing.T, got bool, _ ...any) {
	t.Helper()
	if got {
		t.Fatal("got true, want false")
	}
}

func (testAssert) Contains(t *testing.T, got, want string, _ ...any) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("%q does not contain %q", got, want)
	}
}

func TestGetPath(t *testing.T) {
	cfg := &Config{
		Service: ServiceConfig{
			Name:         "test-gw",
			TickInterval: 10 * time.Second,
		},
		Plugins: map[string]PluginConf{
			"echo": {
				Enabled: true,
				Schedules: []ScheduleConfig{
					{Every: "5m"},
				},
			},
		},
	}

	tests := []struct {
		name    string
		path    string
		want    any
		wantErr bool
	}{
		{
			name: "root service field",
			path: "service.name",
			want: "test-gw",
		},
		{
			name: "nested plugin field",
			path: "plugins.echo.enabled",
			want: true,
		},
		{
			name:    "invalid path",
			path:    "service.missing",
			wantErr: true,
		},
		{
			name: "type:name addressing",
			path: "plugin:echo",
			want: cfg.Plugins["echo"],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cfg.GetPath(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// JSON unmarshal converts time.Duration to int64/float64 usually
				// but for strings and bools it matches perfectly.
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestGetEntity(t *testing.T) {
	cfg := &Config{
		Plugins: map[string]PluginConf{
			"echo":   {Enabled: true},
			"fabric": {Enabled: false},
		},
	}

	t.Run("single plugin", func(t *testing.T) {
		got, err := cfg.GetEntity("plugin:echo")
		assert.NoError(t, err)
		assert.Equal(t, cfg.Plugins["echo"], got)
	})

	t.Run("wildcard plugins", func(t *testing.T) {
		got, err := cfg.GetEntity("plugin:*")
		assert.NoError(t, err)
		assert.Equal(t, cfg.Plugins, got)
	})

	t.Run("unknown plugin", func(t *testing.T) {
		_, err := cfg.GetEntity("plugin:missing")
		assert.Error(t, err)
	})
}

func TestSetPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	initialYAML := `
service:
  name: old-name
plugin_roots:
  - ./plugins
plugins:
  echo:
    enabled: true
    schedules:
      - id: default
        every: 5m
`
	err := os.WriteFile(configPath, []byte(initialYAML), 0644)
	assert.NoError(t, err)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	t.Run("set root field", func(t *testing.T) {
		err := cfg.SetPath("service.name", "new-name", true)
		assert.NoError(t, err)

		// Reload and verify
		reloaded, _ := Load(configPath)
		assert.Equal(t, "new-name", reloaded.Service.Name)
	})

	t.Run("set nested plugin field via entity", func(t *testing.T) {
		err := cfg.SetPath("plugin:echo.enabled", "false", true)
		assert.NoError(t, err)

		// Reload and verify
		reloaded, err := Load(configPath)
		if err != nil {
			t.Fatalf("Load reloaded failed: %v", err)
		}
		assert.False(t, reloaded.Plugins["echo"].Enabled)
	})
}

func TestSetPathRollbackOnValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	initialYAML := `
service:
  name: test-gw
plugin_roots:
  - ./plugins
plugins:
  echo:
    enabled: true
    schedules:
      - id: default
        every: 5m
`
	err := os.WriteFile(configPath, []byte(initialYAML), 0644)
	assert.NoError(t, err)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	err = cfg.SetPath("service.log_level", "invalid", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")

	// File should remain valid and unchanged for this field.
	reloaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load reloaded failed: %v", err)
	}
	assert.Equal(t, "info", reloaded.Service.LogLevel)
}
