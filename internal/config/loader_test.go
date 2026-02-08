package config

import (
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
