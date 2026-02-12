package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetPath(t *testing.T) {
	cfg := &Config{
		Service: ServiceConfig{
			Name:         "test-gw",
			TickInterval: 10 * time.Second,
		},
		Plugins: map[string]PluginConf{
			"echo": {
				Enabled: true,
				Schedule: &ScheduleConfig{
					Every: "5m",
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
			name: "deep schedule field",
			path: "plugins.echo.schedule.every",
			want: "5m",
		},
		{
			name: "invalid path",
			path: "service.missing",
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
			"echo": {Enabled: true},
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
