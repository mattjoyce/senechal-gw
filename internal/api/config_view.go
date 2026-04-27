package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/router/conditions"
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
	Enabled        bool                         `json:"enabled"`
	Uses           string                       `json:"uses,omitempty"`
	Schedules      []config.ScheduleConfig      `json:"schedules,omitempty"`
	Config         map[string]any               `json:"config,omitempty"`
	Retry          *config.RetryConfig          `json:"retry,omitempty"`
	Timeouts       *config.TimeoutsConfig       `json:"timeouts,omitempty"`
	CircuitBreaker *config.CircuitBreakerConfig `json:"circuit_breaker,omitempty"`
	Parallelism    int                          `json:"parallelism,omitempty"`
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
	Service        config.ServiceConfig                 `json:"service"`
	API            configViewAPI                        `json:"api"`
	State          config.StateConfig                   `json:"state"`
	Plugins        map[string]configViewPlugin          `json:"plugins"`
	Pipelines      []config.PipelineEntry               `json:"pipelines"`
	Webhooks       []configViewWebhook                  `json:"webhooks,omitempty"`
	Tokens         []configViewToken                    `json:"tokens,omitempty"`
	Routes         []config.RouteConfig                 `json:"routes,omitempty"`
	CompiledRoutes map[string][]configViewCompiledRoute `json:"compiled_routes,omitempty"`
}

// configViewCompiledRoute exposes one compiled route's match-and-dispatch
// shape for operator inspection. Includes Sprint 17's source-plugin selector
// and the entry-route predicate so the richer match shape is visible rather
// than implicit.
type configViewCompiledRoute struct {
	ID          string                             `json:"id"`
	Source      configViewCompiledRouteSource      `json:"source"`
	Destination configViewCompiledRouteDestination `json:"destination"`
}

type configViewCompiledRouteSource struct {
	Trigger      string                `json:"trigger,omitempty"`
	HookSignal   string                `json:"hook_signal,omitempty"`
	SourcePlugin string                `json:"source_plugin,omitempty"`
	Pipeline     string                `json:"pipeline,omitempty"`
	StepID       string                `json:"step_id,omitempty"`
	EventType    string                `json:"event_type,omitempty"`
	DepthLT      int                   `json:"depth_lt,omitempty"`
	If           *conditions.Condition `json:"if,omitempty"`
}

type configViewCompiledRouteDestination struct {
	Kind         string `json:"kind"`
	StepID       string `json:"step_id,omitempty"`
	Plugin       string `json:"plugin,omitempty"`
	Command      string `json:"command,omitempty"`
	CallPipeline string `json:"call_pipeline,omitempty"`
}

// renderCompiledRoutes walks the router's compiled-route manifest for each
// loaded pipeline and returns a JSON-friendly view. Operators can use this
// to answer "what signal does this route match?", "does it require a source
// plugin?", and "what predicate is evaluated?" without reading the compiled
// DAG by hand.
//
// Pipeline names come from PipelineSummary so the inspection surface works
// in both directory mode (cfg.Pipelines populated) and inline-include mode
// (cfg.Pipelines is nil because pipelines come from a single included file
// rather than a pipelines/ directory).
func renderCompiledRoutes(router PipelineRouter) map[string][]configViewCompiledRoute {
	if router == nil {
		return nil
	}
	summary := router.PipelineSummary()
	if len(summary) == 0 {
		return nil
	}
	out := make(map[string][]configViewCompiledRoute)
	for _, info := range summary {
		routes := router.GetCompiledRoutes(info.Name)
		if len(routes) == 0 {
			continue
		}
		view := make([]configViewCompiledRoute, 0, len(routes))
		for _, route := range routes {
			view = append(view, configViewCompiledRoute{
				ID: route.ID,
				Source: configViewCompiledRouteSource{
					Trigger:      route.Source.Trigger,
					HookSignal:   route.Source.HookSignal,
					SourcePlugin: route.Source.SourcePlugin,
					Pipeline:     route.Source.Pipeline,
					StepID:       route.Source.StepID,
					EventType:    route.Source.EventType,
					DepthLT:      route.Source.DepthLT,
					If:           route.Source.If,
				},
				Destination: configViewCompiledRouteDestination{
					Kind:         string(route.Destination.Kind),
					StepID:       route.Destination.StepID,
					Plugin:       route.Destination.Plugin,
					Command:      route.Destination.Command,
					CallPipeline: route.Destination.CallPipeline,
				},
			})
		}
		out[info.Name] = view
	}
	return out
}

type configViewAPI struct {
	Enabled bool                 `json:"enabled"`
	Listen  string               `json:"listen"`
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
		State:          cfg.State,
		Plugins:        plugins,
		Pipelines:      cfg.Pipelines,
		Webhooks:       webhooks,
		Tokens:         tokens,
		Routes:         cfg.Routes,
		CompiledRoutes: renderCompiledRoutes(s.router),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
