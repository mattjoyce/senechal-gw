package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mattjoyce/ductile/internal/config"
)

// sensitiveKeyPatterns are substrings that flag a plugin config key as sensitive.
var sensitiveKeyPatterns = []string{"secret", "key", "token", "password", "api_key", "credential", "auth", "passwd"}

func isSensitiveKey(k string) bool {
	lower := strings.ToLower(k)
	for _, p := range sensitiveKeyPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// sanitizePluginConfig redacts sensitive values from a plugin's config map.
func sanitizePluginConfig(cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if isSensitiveKey(k) {
			out[k] = "[REDACTED]"
		} else {
			out[k] = v
		}
	}
	return out
}

// -- Sanitized view types ----------------------------------------------------

type configViewPlugin struct {
	Enabled        bool                        `json:"enabled"`
	Uses           string                      `json:"uses,omitempty"`
	Schedules      []config.ScheduleConfig     `json:"schedules,omitempty"`
	Config         map[string]any              `json:"config,omitempty"`
	Retry          *config.RetryConfig         `json:"retry,omitempty"`
	Timeouts       *config.TimeoutsConfig      `json:"timeouts,omitempty"`
	CircuitBreaker *config.CircuitBreakerConfig `json:"circuit_breaker,omitempty"`
	Parallelism    int                         `json:"parallelism,omitempty"`
}

type configViewToken struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	// Key is intentionally omitted.
}

type configViewAPIToken struct {
	Scopes []string `json:"scopes"`
	// Token value is intentionally omitted.
}

type configViewWebhook struct {
	Name            string `json:"name,omitempty"`
	Path            string `json:"path"`
	Plugin          string `json:"plugin"`
	SecretRef       string `json:"secret_ref,omitempty"`
	SignatureHeader string `json:"signature_header,omitempty"`
	MaxBodySize     string `json:"max_body_size,omitempty"`
}

type configViewResponse struct {
	Service   config.ServiceConfig        `json:"service"`
	API       configViewAPI               `json:"api"`
	State     config.StateConfig          `json:"state"`
	Workspace config.WorkspaceConfig      `json:"workspace"`
	Plugins   map[string]configViewPlugin `json:"plugins"`
	Pipelines []config.PipelineEntry      `json:"pipelines"`
	Webhooks  []configViewWebhook         `json:"webhooks,omitempty"`
	Tokens    []configViewToken           `json:"tokens,omitempty"`
	Routes    []config.RouteConfig        `json:"routes,omitempty"`
}

type configViewAPI struct {
	Enabled bool               `json:"enabled"`
	Listen  string             `json:"listen"`
	Tokens  []configViewAPIToken `json:"tokens,omitempty"`
}

// -- Handler -----------------------------------------------------------------

// handleConfigView returns the reconciled config with secrets redacted.
func (s *Server) handleConfigView(w http.ResponseWriter, r *http.Request) {
	cfg := s.config.RuntimeConfig
	if cfg == nil {
		s.writeError(w, http.StatusServiceUnavailable, "config not available")
		return
	}

	// Sanitize API tokens — show scopes only, drop token values.
	apiTokens := make([]configViewAPIToken, 0, len(cfg.API.Auth.Tokens))
	for _, t := range cfg.API.Auth.Tokens {
		apiTokens = append(apiTokens, configViewAPIToken{Scopes: t.Scopes})
	}

	// Sanitize plugins — redact sensitive config map keys.
	plugins := make(map[string]configViewPlugin, len(cfg.Plugins))
	for name, p := range cfg.Plugins {
		schedules := p.Schedules
		if schedules == nil && p.Schedule != nil {
			schedules = []config.ScheduleConfig{*p.Schedule}
		}
		plugins[name] = configViewPlugin{
			Enabled:        p.Enabled,
			Uses:           p.Uses,
			Schedules:      schedules,
			Config:         sanitizePluginConfig(p.Config),
			Retry:          p.Retry,
			Timeouts:       p.Timeouts,
			CircuitBreaker: p.CircuitBreaker,
			Parallelism:    p.Parallelism,
		}
	}

	// Sanitize webhooks — SecretRef is a name reference, not the secret value; safe.
	var webhooks []configViewWebhook
	if cfg.Webhooks != nil {
		for _, ep := range cfg.Webhooks.Endpoints {
			webhooks = append(webhooks, configViewWebhook{
				Name:            ep.Name,
				Path:            ep.Path,
				Plugin:          ep.Plugin,
				SecretRef:       ep.SecretRef,
				SignatureHeader: ep.SignatureHeader,
				MaxBodySize:     ep.MaxBodySize,
			})
		}
	}

	// Sanitize tokens — drop Key field.
	tokens := make([]configViewToken, 0, len(cfg.Tokens))
	for _, t := range cfg.Tokens {
		tokens = append(tokens, configViewToken{
			Name:        t.Name,
			Description: t.Description,
			CreatedAt:   t.CreatedAt,
		})
	}

	resp := configViewResponse{
		Service: cfg.Service,
		API: configViewAPI{
			Enabled: cfg.API.Enabled,
			Listen:  cfg.API.Listen,
			Tokens:  apiTokens,
		},
		State:     cfg.State,
		Workspace: cfg.Workspace,
		Plugins:   plugins,
		Pipelines: cfg.Pipelines,
		Webhooks:  webhooks,
		Tokens:    tokens,
		Routes:    cfg.Routes,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
