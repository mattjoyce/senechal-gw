package api

import (
	"encoding/json"
	"time"
)

// TriggerRequest is the JSON body for POST /trigger/{plugin}/{command}
type TriggerRequest struct {
	Payload json.RawMessage `json:"payload,omitempty"`
}

// TriggerResponse is returned on successful job enqueue
type TriggerResponse struct {
	JobID   string `json:"job_id"`
	Status  string `json:"status"`
	Plugin  string `json:"plugin"`
	Command string `json:"command"`
}

// JobStatusResponse is returned by GET /job/{job_id}
type JobStatusResponse struct {
	JobID       string          `json:"job_id"`
	Status      string          `json:"status"`
	Plugin      string          `json:"plugin"`
	Command     string          `json:"command"`
	Result      json.RawMessage `json:"result,omitempty"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// ErrorResponse is returned on errors
type ErrorResponse struct {
	Error string `json:"error"`
}

// HealthzResponse is returned by GET /healthz.
type HealthzResponse struct {
	Status        string `json:"status"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	QueueDepth    int    `json:"queue_depth"`
	PluginsLoaded int    `json:"plugins_loaded"`
	// PluginsCircuitOpen is reserved for circuit breaker observability.
	// MVP: always 0 until circuit breaker state is plumbed into the API server.
	PluginsCircuitOpen int `json:"plugins_circuit_open"`
}
