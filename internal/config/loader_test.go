package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		env     map[string]string
		wantErr bool
		checkFn func(t *testing.T, cfg *Config)
	}{
		{
			name: "minimal valid config",
			yaml: `
service:
  tick_interval: 60s
state:
  path: ./test.db
plugins_dir: ./plugins
plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
      jitter: 30s
`,
			wantErr: false,
			checkFn: func(t *testing.T, cfg *Config) {
				if cfg.Service.TickInterval != 60*time.Second {
					t.Error("tick_interval not parsed")
				}
				if cfg.State.Path != "./test.db" {
					t.Error("state.path not parsed")
				}
				if cfg.PluginsDir != "./plugins" {
					t.Error("plugins_dir not parsed")
				}
				echo, ok := cfg.Plugins["echo"]
				if !ok {
					t.Fatal("echo plugin not found")
				}
				if !echo.Enabled {
					t.Error("echo not enabled")
				}
				if echo.Schedule.Every != "5m" {
					t.Error("schedule.every not parsed")
				}
				// Check defaults applied
				if echo.Retry == nil || echo.Retry.MaxAttempts != 4 {
					t.Error("default retry config not applied")
				}
			},
		},
		{
			name: "env var interpolation",
			yaml: `
service:
  tick_interval: 30s
state:
  path: ${DB_PATH}
plugins_dir: ./plugins
plugins:
  test:
    enabled: true
    schedule:
      every: hourly
    config:
      api_key: ${API_KEY}
      endpoint: ${API_ENDPOINT}
`,
			env: map[string]string{
				"DB_PATH":      "/tmp/test.db",
				"API_KEY":      "secret123",
				"API_ENDPOINT": "https://api.example.com",
			},
			wantErr: false,
			checkFn: func(t *testing.T, cfg *Config) {
				if cfg.State.Path != "/tmp/test.db" {
					t.Errorf("env var not interpolated in state.path: %s", cfg.State.Path)
				}
				test := cfg.Plugins["test"]
				if test.Config["api_key"] != "secret123" {
					t.Error("env var not interpolated in plugin config")
				}
				if test.Config["endpoint"] != "https://api.example.com" {
					t.Error("env var not interpolated in plugin config")
				}
			},
		},
		{
			name: "missing env var fails validation",
			yaml: `
service:
  tick_interval: 30s
state:
  path: ./test.db
plugins_dir: ./plugins
plugins:
  test:
    enabled: true
    schedule:
      every: daily
    config:
      secret: ${MISSING_VAR}
`,
			env:     map[string]string{}, // MISSING_VAR not set
			wantErr: true,
		},
		{
			name: "invalid log level",
			yaml: `
service:
  tick_interval: 30s
  log_level: invalid
state:
  path: ./test.db
plugins_dir: ./plugins
`,
			wantErr: true,
		},
		{
			name: "missing required schedule",
			yaml: `
service:
  tick_interval: 30s
state:
  path: ./test.db
plugins_dir: ./plugins
plugins:
  test:
    enabled: true
`,
			wantErr: true,
		},
		{
			name: "invalid schedule interval",
			yaml: `
service:
  tick_interval: 30s
state:
  path: ./test.db
plugins_dir: ./plugins
plugins:
  test:
    enabled: true
    schedule:
      every: invalid
`,
			wantErr: true,
		},
		{
			name: "disabled plugin skips validation",
			yaml: `
service:
  tick_interval: 30s
state:
  path: ./test.db
plugins_dir: ./plugins
plugins:
  test:
    enabled: false
`,
			wantErr: false,
			checkFn: func(t *testing.T, cfg *Config) {
				test := cfg.Plugins["test"]
				if test.Enabled {
					t.Error("plugin should be disabled")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			for k, v := range tt.env {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			// Create temp config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.yaml), 0644); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			// Load config
			cfg, err := Load(configPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFn != nil {
				tt.checkFn(t, cfg)
			}
		})
	}
}

func TestInterpolateEnv(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		env    map[string]string
		want   string
	}{
		{
			name:  "simple replacement",
			input: "path: ${HOME}/data",
			env:   map[string]string{"HOME": "/users/test"},
			want:  "path: /users/test/data",
		},
		{
			name:  "multiple vars",
			input: "${USER}:${PASS}@${HOST}",
			env: map[string]string{
				"USER": "admin",
				"PASS": "secret",
				"HOST": "localhost",
			},
			want: "admin:secret@localhost",
		},
		{
			name:  "undefined var unchanged",
			input: "key: ${UNDEFINED}",
			env:   map[string]string{},
			want:  "key: ${UNDEFINED}",
		},
		{
			name:  "no vars",
			input: "plain text",
			env:   map[string]string{},
			want:  "plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			for k, v := range tt.env {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			got := interpolateEnv(tt.input)
			if got != tt.want {
				t.Errorf("interpolateEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseInterval(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"5 minutes", "5m", 5 * time.Minute, false},
		{"15 minutes", "15m", 15 * time.Minute, false},
		{"30 minutes", "30m", 30 * time.Minute, false},
		{"hourly", "hourly", 1 * time.Hour, false},
		{"2 hours", "2h", 2 * time.Hour, false},
		{"6 hours", "6h", 6 * time.Hour, false},
		{"daily", "daily", 0, false},    // Special case
		{"weekly", "weekly", 0, false},  // Special case
		{"monthly", "monthly", 0, false}, // Special case
		{"invalid", "invalid", 0, true},
		{"negative", "-5m", 0, true},
		{"zero", "0s", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInterval(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseInterval() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				Service: ServiceConfig{
					TickInterval: 60 * time.Second,
					LogLevel:     "info",
				},
				State:      StateConfig{Path: "./test.db"},
				PluginsDir: "./plugins",
				Plugins: map[string]PluginConf{
					"test": {
						Enabled: true,
						Schedule: &ScheduleConfig{
							Every: "hourly",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "negative tick interval",
			cfg: &Config{
				Service:    ServiceConfig{TickInterval: -1},
				State:      StateConfig{Path: "./test.db"},
				PluginsDir: "./plugins",
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			cfg: &Config{
				Service: ServiceConfig{
					TickInterval: 60 * time.Second,
					LogLevel:     "trace",
				},
				State:      StateConfig{Path: "./test.db"},
				PluginsDir: "./plugins",
			},
			wantErr: true,
		},
		{
			name: "missing state path",
			cfg: &Config{
				Service:    ServiceConfig{TickInterval: 60 * time.Second, LogLevel: "info"},
				State:      StateConfig{},
				PluginsDir: "./plugins",
			},
			wantErr: true,
		},
		{
			name: "missing plugins dir",
			cfg: &Config{
				Service: ServiceConfig{TickInterval: 60 * time.Second, LogLevel: "info"},
				State:   StateConfig{Path: "./test.db"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestLoadMultiFileWithInclude tests loading configuration using include array.
func TestLoadMultiFileWithInclude(t *testing.T) {
	tmpDir := t.TempDir()

	// Create main config.yaml with include array
	configYAML := `
include:
  - plugins.yaml

service:
  name: senechal-gw
  tick_interval: 60s
  log_level: info
state:
  path: /var/lib/senechal-gw/state.db
plugins_dir: /usr/local/lib/senechal-gw/plugins
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create plugins.yaml
	pluginsYAML := `
plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
      jitter: 30s
`
	if err := os.WriteFile(filepath.Join(tmpDir, "plugins.yaml"), []byte(pluginsYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Load config
	configPath := filepath.Join(tmpDir, "config.yaml")
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify config loaded correctly
	if cfg.Service.Name != "senechal-gw" {
		t.Errorf("Service.Name = %q, want %q", cfg.Service.Name, "senechal-gw")
	}
	if cfg.State.Path != "/var/lib/senechal-gw/state.db" {
		t.Errorf("State.Path = %q", cfg.State.Path)
	}
	if _, ok := cfg.Plugins["echo"]; !ok {
		t.Error("echo plugin not loaded from included file")
	}
}

// TestLoadMissingIncludeFile tests hard fail when included file is missing.
func TestLoadMissingIncludeFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config.yaml with include to non-existent file
	configYAML := `
include:
  - missing.yaml

service:
  tick_interval: 60s
state:
  path: ./test.db
plugins_dir: ./plugins
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Load should fail with good error message
	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() should have failed with missing include file")
	}

	// Check error message is helpful
	errMsg := err.Error()
	if !contains(errMsg, "file not found") {
		t.Errorf("Error message should mention 'file not found', got: %v", errMsg)
	}
	if !contains(errMsg, "Hint") {
		t.Errorf("Error message should contain hint, got: %v", errMsg)
	}
}

// TestLoadAbsoluteIncludePath tests include with absolute path.
func TestLoadAbsoluteIncludePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create plugins.yaml in temp dir
	pluginsYAML := `
plugins:
  test:
    enabled: true
    schedule:
      every: 5m
`
	pluginsPath := filepath.Join(tmpDir, "plugins.yaml")
	if err := os.WriteFile(pluginsPath, []byte(pluginsYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create config.yaml with absolute path include
	configYAML := fmt.Sprintf(`
include:
  - %s

service:
  tick_interval: 60s
state:
  path: ./test.db
plugins_dir: ./plugins
`, pluginsPath)

	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Load should succeed
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if _, ok := cfg.Plugins["test"]; !ok {
		t.Error("test plugin not loaded from absolute path include")
	}
}

// TestLoadCircularInclude tests cycle detection in includes.
func TestLoadCircularInclude(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a.yaml that includes b.yaml
	aYAML := `
include:
  - b.yaml
service:
  tick_interval: 60s
`
	if err := os.WriteFile(filepath.Join(tmpDir, "a.yaml"), []byte(aYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create b.yaml that includes a.yaml (circular)
	bYAML := `
include:
  - a.yaml
state:
  path: ./test.db
`
	if err := os.WriteFile(filepath.Join(tmpDir, "b.yaml"), []byte(bYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create main config
	configYAML := `
include:
  - a.yaml
plugins_dir: ./plugins
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Load should fail with cycle detection
	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() should have failed with circular dependency")
	}

	if !contains(err.Error(), "circular dependency") {
		t.Errorf("Error should mention circular dependency, got: %v", err)
	}
}

// contains checks if a string contains a substring (helper for tests).
func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) &&
		(func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		})()
}

// TestHashVerification tests BLAKE3 hash verification for scope files.
func TestHashVerification(t *testing.T) {
	tmpDir := t.TempDir()

	// Create tokens.yaml (scope file)
	tokensYAML := `
api_key: original-secret
`
	if err := os.WriteFile(filepath.Join(tmpDir, "tokens.yaml"), []byte(tokensYAML), 0600); err != nil {
		t.Fatal(err)
	}

	// Create config with include for tokens.yaml
	configYAML := `
include:
  - tokens.yaml

service:
  tick_interval: 60s
state:
  path: ./test.db
plugins_dir: ./plugins
plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Generate checksums for tokens.yaml
	if err := GenerateChecksums(tmpDir, []string{"tokens.yaml"}); err != nil {
		t.Fatal(err)
	}

	// Load should succeed with correct hash
	if _, err := Load(configPath); err != nil {
		t.Fatalf("Load() with correct hash failed: %v", err)
	}

	// Modify tokens.yaml (tamper with it)
	tamperedYAML := `
api_key: tampered-secret
`
	if err := os.WriteFile(filepath.Join(tmpDir, "tokens.yaml"), []byte(tamperedYAML), 0600); err != nil {
		t.Fatal(err)
	}

	// Load should fail with hash mismatch
	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() should have failed with hash mismatch")
	}

	// Check error message is helpful
	if !contains(err.Error(), "verification failed") {
		t.Errorf("Error should mention verification failed, got: %v", err)
	}
}

// TestDeepMerge tests deep merging of included configs.
func TestDeepMerge(t *testing.T) {
	tmpDir := t.TempDir()

	// Create base.yaml with some defaults
	baseYAML := `
service:
  name: senechal-gw
  tick_interval: 60s
  log_level: info
`
	if err := os.WriteFile(filepath.Join(tmpDir, "base.yaml"), []byte(baseYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create override.yaml that overrides log_level
	overrideYAML := `
service:
  log_level: debug
`
	if err := os.WriteFile(filepath.Join(tmpDir, "override.yaml"), []byte(overrideYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create main config that includes both
	configYAML := `
include:
  - base.yaml
  - override.yaml

state:
  path: ./test.db
plugins_dir: ./plugins
plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Load config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify merge: should have name from base, log_level from override
	if cfg.Service.Name != "senechal-gw" {
		t.Errorf("Service.Name = %q, want %q", cfg.Service.Name, "senechal-gw")
	}
	if cfg.Service.LogLevel != "debug" {
		t.Errorf("Service.LogLevel = %q, want %q (should be overridden)", cfg.Service.LogLevel, "debug")
	}
	if cfg.Service.TickInterval != 60*time.Second {
		t.Error("Service.TickInterval should be preserved from base")
	}
}

// TestLegacySingleFile ensures backward compatibility with single config.yaml.
func TestLegacySingleFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a single config.yaml with all settings
	legacyYAML := `
service:
  tick_interval: 60s
  log_level: info
state:
  path: ./test.db
plugins_dir: ./plugins
plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(legacyYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Load should work with single file (legacy mode)
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() legacy mode failed: %v", err)
	}

	// Verify config loaded correctly
	if cfg.Service.TickInterval != 60*time.Second {
		t.Error("tick_interval not parsed in legacy mode")
	}
	if _, ok := cfg.Plugins["echo"]; !ok {
		t.Error("echo plugin not loaded in legacy mode")
	}
}
