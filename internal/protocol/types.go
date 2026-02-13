package protocol

import "time"

// Request represents the protocol v2 request envelope sent to plugins via stdin.
type Request struct {
	Protocol     int            `json:"protocol"`
	JobID        string         `json:"job_id"`
	Command      string         `json:"command"` // poll | handle | health | init
	Config       map[string]any `json:"config"`
	State        map[string]any `json:"state"`
	Context      map[string]any `json:"context,omitempty"`
	WorkspaceDir string         `json:"workspace_dir,omitempty"`
	Event        *Event         `json:"event,omitempty"` // only for handle command
	DeadlineAt   time.Time      `json:"deadline_at"`
}

// Response represents the protocol v2 response envelope received from plugins via stdout.
type Response struct {
	Status       string         `json:"status"` // ok | error
	Error        string         `json:"error,omitempty"`
	Retry        *bool          `json:"retry,omitempty"` // defaults to true if omitted
	Events       []Event        `json:"events,omitempty"`
	StateUpdates map[string]any `json:"state_updates,omitempty"`
	Logs         []LogEntry     `json:"logs,omitempty"`
}

// Event represents an event emitted by a plugin or received via webhook.
type Event struct {
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	DedupeKey string         `json:"dedupe_key,omitempty"`

	// Injected by core (not set by plugins)
	Source    string    `json:"source,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
	EventID   string    `json:"event_id,omitempty"`
}

// LogEntry represents a log message from a plugin.
type LogEntry struct {
	Level   string `json:"level"` // info | warn | error | debug
	Message string `json:"message"`
}

// ShouldRetry returns true if the response indicates the job should be retried.
// Defaults to true if retry field is omitted.
func (r *Response) ShouldRetry() bool {
	if r.Retry == nil {
		return true
	}
	return *r.Retry
}
