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

// SyncResponse is returned for successful synchronous pipeline executions.
type SyncResponse struct {
	JobID      string          `json:"job_id"`
	Status     string          `json:"status"`
	DurationMs int64           `json:"duration_ms"`
	Result     json.RawMessage `json:"result,omitempty"` // Root job result
	Tree       []JobResultData `json:"tree"`             // All jobs in tree
}

// JobResultData represents a single job's result in a tree.
type JobResultData struct {
	JobID       string          `json:"job_id"`
	Plugin      string          `json:"plugin"`
	Command     string          `json:"command"`
	Status      string          `json:"status"`
	Result      json.RawMessage `json:"result,omitempty"`
	LastError   *string         `json:"last_error,omitempty"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// TimeoutResponse is returned when a sync pipeline exceeds its timeout.
type TimeoutResponse struct {
	JobID           string `json:"job_id"`
	Status          string `json:"status"`
	TimeoutExceeded bool   `json:"timeout_exceeded"`
	Message         string `json:"message"`
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
