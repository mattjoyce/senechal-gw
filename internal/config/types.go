package config

import (
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete ductile configuration.
type Config struct {
	Include     []string              `yaml:"include,omitempty"` // Multi-file mode: files to merge
	Service     ServiceConfig         `yaml:"service"`
	State       StateConfig           `yaml:"state"`
	Database    StateConfig           `yaml:"database,omitempty"` // Alias for user intuition
	API         APIConfig             `yaml:"api,omitempty"`
	PluginsDir  string                `yaml:"plugins_dir"`
	Plugins     map[string]PluginConf `yaml:"plugins"`
	Routes      []RouteConfig         `yaml:"routes,omitempty"`   // Not in MVP
	Webhooks    *WebhooksConfig       `yaml:"webhooks,omitempty"` // Not in MVP
	SourceFiles map[string]*yaml.Node `yaml:"-"`                  // Physical files tracked for updates
	Tokens      []TokenEntry          `yaml:"-"`                  // Directory mode: token entries from tokens.yaml
	Pipelines   []PipelineEntry       `yaml:"-"`                  // Directory mode: pipeline entries
	ConfigDir   string                `yaml:"-"`                  // Directory mode: root config directory
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
	Enabled           bool          `yaml:"enabled"`
	Listen            string        `yaml:"listen"`
	Auth              APIAuthConfig `yaml:"auth"`
	MaxConcurrentSync int           `yaml:"max_concurrent_sync,omitempty"`
	MaxSyncTimeout    time.Duration `yaml:"max_sync_timeout,omitempty"`
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
	Name            string `yaml:"name,omitempty"`       // Directory mode: endpoint name
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

// TokenEntry defines an API token with scoped permissions (directory mode tokens.yaml).
type TokenEntry struct {
	Name       string `yaml:"name"`
	Key        string `yaml:"key"`
	ScopesFile string `yaml:"scopes_file,omitempty"`
	ScopesHash string `yaml:"scopes_hash,omitempty"`
}

// TokensFileConfig wraps token entries for standalone tokens.yaml.
type TokensFileConfig struct {
	Tokens []TokenEntry `yaml:"tokens"`
}

// WebhooksFileConfig wraps webhook endpoints for standalone webhooks.yaml.
type WebhooksFileConfig struct {
	Webhooks []WebhookEndpoint `yaml:"webhooks"`
}

// ExecutionMode defines how a pipeline should be triggered and its results returned.
type ExecutionMode string

const (
	// ExecutionModeAsync returns 202 immediately.
	ExecutionModeAsync ExecutionMode = "async"
	// ExecutionModeSync blocks until the pipeline completes or times out.
	ExecutionModeSync ExecutionMode = "synchronous"
)

// PipelineEntry defines a named pipeline triggered by an event type.
type PipelineEntry struct {
	Name          string        `yaml:"name"`
	On            string        `yaml:"on"`
	Steps         []StepEntry   `yaml:"steps,omitempty"`
	ExecutionMode ExecutionMode `yaml:"execution_mode,omitempty"`
	Timeout       time.Duration `yaml:"timeout,omitempty"`
}

// StepEntry is a single step in a pipeline.
type StepEntry struct {
	ID    string      `yaml:"id,omitempty"`
	Uses  string      `yaml:"uses,omitempty"`
	Call  string      `yaml:"call,omitempty"`
	Steps []StepEntry `yaml:"steps,omitempty"`
	Split []StepEntry `yaml:"split,omitempty"`
}

// PipelinesFileConfig wraps pipeline entries for standalone pipelines/*.yaml.
type PipelinesFileConfig struct {
	Pipelines []PipelineEntry `yaml:"pipelines"`
}

// IntegrityTier classifies files by security sensitivity.
type IntegrityTier int

const (
	// TierOperational files warn on mismatch but allow loading.
	TierOperational IntegrityTier = iota
	// TierHighSecurity files hard-fail on mismatch.
	TierHighSecurity
)

// IntegrityResult captures the outcome of integrity verification.
type IntegrityResult struct {
	Passed   bool
	Warnings []string
	Errors   []string
}

// ConfigFiles represents the discovered file manifest for directory mode.
type ConfigFiles struct {
	Root      string   // Config directory root (absolute)
	Config    string   // config.yaml path (absolute)
	Plugins   []string // plugins/*.yaml paths (absolute, sorted)
	Pipelines []string // pipelines/*.yaml paths (absolute, sorted)
	Webhooks  string   // webhooks.yaml path (absolute, empty if missing)
	Tokens    string   // tokens.yaml path (absolute, empty if missing)
	Routes    string   // routes.yaml path (absolute, empty if missing)
	Scopes    []string // scopes/*.json paths (absolute, sorted)
}

// FileTier returns the integrity tier for a given file path.
func (cf *ConfigFiles) FileTier(path string) IntegrityTier {
	if path == cf.Tokens || path == cf.Webhooks {
		return TierHighSecurity
	}
	for _, s := range cf.Scopes {
		if path == s {
			return TierHighSecurity
		}
	}
	return TierOperational
}

// AllFiles returns all discovered file paths.
func (cf *ConfigFiles) AllFiles() []string {
	var files []string
	files = append(files, cf.Config)
	files = append(files, cf.Plugins...)
	files = append(files, cf.Pipelines...)
	if cf.Webhooks != "" {
		files = append(files, cf.Webhooks)
	}
	if cf.Tokens != "" {
		files = append(files, cf.Tokens)
	}
	if cf.Routes != "" {
		files = append(files, cf.Routes)
	}
	files = append(files, cf.Scopes...)
	return files
}

// HighSecurityFiles returns only high-security tier file paths.
func (cf *ConfigFiles) HighSecurityFiles() []string {
	var files []string
	if cf.Tokens != "" {
		files = append(files, cf.Tokens)
	}
	if cf.Webhooks != "" {
		files = append(files, cf.Webhooks)
	}
	files = append(files, cf.Scopes...)
	return files
}

// OperationalFiles returns only operational tier file paths.
func (cf *ConfigFiles) OperationalFiles() []string {
	var files []string
	files = append(files, cf.Config)
	files = append(files, cf.Plugins...)
	files = append(files, cf.Pipelines...)
	if cf.Routes != "" {
		files = append(files, cf.Routes)
	}
	return files
}

// Defaults returns a Config with sensible defaults for MVP.
func Defaults() *Config {
	return &Config{
		Service: ServiceConfig{
			Name:            "ductile",
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
