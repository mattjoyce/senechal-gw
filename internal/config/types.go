package config

import "time"

// Config represents the complete senechal-gw configuration.
type Config struct {
	Include    []string              `yaml:"include,omitempty"`  // Multi-file mode: files to merge
	Service    ServiceConfig         `yaml:"service"`
	State      StateConfig           `yaml:"state"`
	API        APIConfig             `yaml:"api,omitempty"`
	PluginsDir string                `yaml:"plugins_dir"`
	Plugins    map[string]PluginConf `yaml:"plugins"`
	Routes     []RouteConfig         `yaml:"routes,omitempty"`   // Not in MVP
	Webhooks   *WebhooksConfig       `yaml:"webhooks,omitempty"` // Not in MVP
}

// ServiceConfig defines core service settings.
type ServiceConfig struct {
	Name            string        `yaml:"name"`
	TickInterval    time.Duration `yaml:"tick_interval"`
	LogLevel        string        `yaml:"log_level"`
	LogFormat       string        `yaml:"log_format"`
	DedupeTTL       time.Duration `yaml:"dedupe_ttl"`
	JobLogRetention time.Duration `yaml:"job_log_retention"`
}

// StateConfig defines state storage settings.
type StateConfig struct {
	Path string `yaml:"path"`
}

// APIConfig defines HTTP API server settings.
type APIConfig struct {
	Enabled bool          `yaml:"enabled"`
	Listen  string        `yaml:"listen"`
	Auth    APIAuthConfig `yaml:"auth"`
}

// APIAuthConfig defines API authentication settings.
type APIAuthConfig struct {
	// APIKey is the legacy single bearer token (admin/full access).
	// Prefer Tokens for scoped access.
	APIKey string     `yaml:"api_key"`
	Tokens []APIToken `yaml:"tokens,omitempty"`
}

// APIToken defines a bearer token and its scopes.
type APIToken struct {
	Token  string   `yaml:"token"`
	Scopes []string `yaml:"scopes"`
}

// PluginConf defines configuration for a single plugin.
type PluginConf struct {
	Enabled             bool                   `yaml:"enabled"`
	Schedule            *ScheduleConfig        `yaml:"schedule,omitempty"`
	Config              map[string]interface{} `yaml:"config,omitempty"`
	Retry               *RetryConfig           `yaml:"retry,omitempty"`
	Timeouts            *TimeoutsConfig        `yaml:"timeouts,omitempty"`
	CircuitBreaker      *CircuitBreakerConfig  `yaml:"circuit_breaker,omitempty"`
	MaxOutstandingPolls int                    `yaml:"max_outstanding_polls,omitempty"`
}

// ScheduleConfig defines when a plugin should be polled.
type ScheduleConfig struct {
	Every           string           `yaml:"every"` // e.g., "5m", "hourly", "daily"
	Jitter          time.Duration    `yaml:"jitter,omitempty"`
	PreferredWindow *PreferredWindow `yaml:"preferred_window,omitempty"` // Not in MVP
}

// PreferredWindow defines time-of-day constraints for scheduling.
type PreferredWindow struct {
	Start string `yaml:"start"` // e.g., "06:00"
	End   string `yaml:"end"`   // e.g., "22:00"
}

// RetryConfig defines retry behavior for failed jobs.
type RetryConfig struct {
	MaxAttempts int           `yaml:"max_attempts"`
	BackoffBase time.Duration `yaml:"backoff_base"`
}

// TimeoutsConfig defines command-specific timeouts.
type TimeoutsConfig struct {
	Poll   time.Duration `yaml:"poll"`
	Handle time.Duration `yaml:"handle"`
	Health time.Duration `yaml:"health,omitempty"`
	Init   time.Duration `yaml:"init,omitempty"`
}

// CircuitBreakerConfig defines circuit breaker settings.
type CircuitBreakerConfig struct {
	Threshold  int           `yaml:"threshold"`
	ResetAfter time.Duration `yaml:"reset_after"`
}

// RouteConfig defines event routing between plugins.
type RouteConfig struct {
	From      string `yaml:"from"`
	EventType string `yaml:"event_type"`
	To        string `yaml:"to"`
}

// WebhooksConfig defines webhook listener settings.
type WebhooksConfig struct {
	Listen    string            `yaml:"listen"`
	Endpoints []WebhookEndpoint `yaml:"endpoints"`
}

// WebhookEndpoint defines a single webhook endpoint.
type WebhookEndpoint struct {
	Path            string `yaml:"path"`
	Plugin          string `yaml:"plugin"`
	Secret          string `yaml:"secret,omitempty"`    // Legacy: direct secret (deprecated)
	SecretRef       string `yaml:"secret_ref,omitempty"` // Preferred: reference to tokens.yaml
	SignatureHeader string `yaml:"signature_header"`
	MaxBodySize     string `yaml:"max_body_size"`
}

// TokensConfig defines sensitive authentication tokens (separate file for security).
type TokensConfig struct {
	Tokens map[string]string `yaml:",inline"` // Flat key-value map
}

// ChecksumManifest stores BLAKE3 hashes for scope files (tokens.yaml, webhooks.yaml).
type ChecksumManifest struct {
	Version     int               `yaml:"version"`
	GeneratedAt string            `yaml:"generated_at"`
	Hashes      map[string]string `yaml:"hashes"` // filename -> BLAKE3 hash
}

// PluginsFileConfig is the structure of plugins.yaml.
type PluginsFileConfig struct {
	Plugins map[string]PluginConf `yaml:"plugins"`
}

// RoutesFileConfig is the structure of routes.yaml.
type RoutesFileConfig struct {
	Routes []RouteConfig `yaml:"routes"`
}

// Defaults returns a Config with sensible defaults for MVP.
func Defaults() *Config {
	return &Config{
		Service: ServiceConfig{
			Name:            "senechal-gw",
			TickInterval:    60 * time.Second,
			LogLevel:        "info",
			LogFormat:       "json",
			DedupeTTL:       24 * time.Hour,
			JobLogRetention: 30 * 24 * time.Hour,
		},
		State: StateConfig{
			Path: "./data/state.db",
		},
		API: APIConfig{
			Enabled: false,
			Listen:  "127.0.0.1:8080",
			Auth: APIAuthConfig{
				APIKey: "",
			},
		},
		PluginsDir: "./plugins",
		Plugins:    make(map[string]PluginConf),
	}
}

// DefaultPluginConf returns default plugin configuration.
func DefaultPluginConf() PluginConf {
	return PluginConf{
		Enabled: true,
		Retry: &RetryConfig{
			MaxAttempts: 4,
			BackoffBase: 30 * time.Second,
		},
		Timeouts: &TimeoutsConfig{
			Poll:   60 * time.Second,
			Handle: 120 * time.Second,
			Health: 10 * time.Second,
			Init:   30 * time.Second,
		},
		CircuitBreaker: &CircuitBreakerConfig{
			Threshold:  3,
			ResetAfter: 30 * time.Minute,
		},
		MaxOutstandingPolls: 1,
	}
}
