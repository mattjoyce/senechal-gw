package types

import (
	"encoding/json"
	"time"
)

// RuntimeHealth mirrors the /healthz API response.
type RuntimeHealth struct {
	Status             string `json:"status"`
	UptimeSeconds      int64  `json:"uptime_seconds"`
	QueueDepth         int    `json:"queue_depth"`
	PluginsLoaded      int    `json:"plugins_loaded"`
	PluginsCircuitOpen int    `json:"plugins_circuit_open"`
	ConfigPath         string `json:"config_path,omitempty"`
	BinaryPath         string `json:"binary_path,omitempty"`
	Version            string `json:"version,omitempty"`
}

// Event represents an SSE event from the /events stream.
type Event struct {
	ID   int64     `json:"id"`
	Type string    `json:"type"`
	At   time.Time `json:"at"`
	Data []byte    `json:"data"`
}

// EventData is the common shape of the JSON inside Event.Data.
type EventData struct {
	At          string `json:"at,omitempty"`
	JobID       string `json:"job_id,omitempty"`
	Plugin      string `json:"plugin,omitempty"`
	Command     string `json:"command,omitempty"`
	SubmittedBy string `json:"submitted_by,omitempty"`
	Pipeline    string `json:"pipeline,omitempty"`
	Status      string `json:"status,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ParseEventData extracts structured fields from raw event data.
func ParseEventData(raw []byte) EventData {
	var d EventData
	_ = json.Unmarshal(raw, &d)
	return d
}

// Job mirrors a single job from the /jobs API.
type Job struct {
	JobID       string     `json:"job_id"`
	Plugin      string     `json:"plugin"`
	Command     string     `json:"command"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Attempt     int        `json:"attempt"`
}

// JobDetail mirrors the /job/{id} API response.
type JobDetail struct {
	JobID       string          `json:"job_id"`
	Status      string          `json:"status"`
	Plugin      string          `json:"plugin"`
	Command     string          `json:"command"`
	Result      json.RawMessage `json:"result,omitempty"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// JobLog mirrors a single log entry from the /job-logs API.
type JobLog struct {
	JobID       string          `json:"job_id"`
	LogID       string          `json:"log_id"`
	Plugin      string          `json:"plugin"`
	Command     string          `json:"command"`
	Status      string          `json:"status"`
	Attempt     int             `json:"attempt"`
	SubmittedBy string          `json:"submitted_by"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt time.Time       `json:"completed_at"`
	LastError   *string         `json:"last_error,omitempty"`
	Stderr      *string         `json:"stderr,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

// SchedulerJob mirrors a single entry from /scheduler/jobs.
type SchedulerJob struct {
	Plugin     string     `json:"plugin"`
	ScheduleID string     `json:"schedule_id"`
	Command    string     `json:"command"`
	Mode       string     `json:"mode"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason,omitempty"`
	Timezone   string     `json:"timezone,omitempty"`
	NextRunAt  *time.Time `json:"next_run_at,omitempty"`
}

// PluginSummary mirrors a single plugin from /plugins.
type PluginSummary struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Commands    []string `json:"commands"`
}

// PluginDetail mirrors /plugin/{name}.
type PluginDetail struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description,omitempty"`
	Protocol    int             `json:"protocol"`
	Commands    []PluginCommand `json:"commands"`
}

// PluginCommand describes a command on a plugin.
type PluginCommand struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// DetailTarget is what the detail screen should render.
type DetailTarget interface {
	detailTarget()
}

// JobTarget opens a job in the detail view.
type JobTarget struct {
	JobID string
}

func (JobTarget) detailTarget() {}

// EventTarget opens an event in the detail view.
type EventTarget struct {
	Event Event
}

func (EventTarget) detailTarget() {}

// PluginTarget opens a plugin in the detail view.
type PluginTarget struct {
	Name string
}

func (PluginTarget) detailTarget() {}
