package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusTimedOut  Status = "timed_out"
	StatusDead      Status = "dead"
)

type Job struct {
	ID             string
	Plugin         string
	Command        string
	Payload        json.RawMessage
	Status         Status
	Attempt        int
	MaxAttempts    int
	SubmittedBy    string
	DedupeKey      *string
	CreatedAt      time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
	NextRetryAt    *time.Time
	LastError      *string
	ParentJobID    *string
	SourceEventID  *string
	EventContextID *string
}

type EnqueueRequest struct {
	Plugin         string
	Command        string
	Payload        json.RawMessage
	MaxAttempts    int
	SubmittedBy    string
	DedupeKey      *string
	ParentJobID    *string
	EventContextID *string
	SourceEventID  *string
}

// ListJobsFilter defines optional filters for listing jobs.
type ListJobsFilter struct {
	Plugin  string
	Command string
	Status  *Status
	Limit   int
}

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type CircuitBreaker struct {
	Plugin       string
	Command      string
	State        CircuitState
	FailureCount int
	OpenedAt     *time.Time
	LastFailure  *time.Time
	LastJobID    *string
	UpdatedAt    time.Time
}

type CommandResult struct {
	JobID       string
	Status      Status
	CompletedAt time.Time
}

// PollResult is kept as an alias for compatibility with existing tests/callers.
type PollResult = CommandResult

type ScheduleEntryStatus string

const (
	ScheduleEntryActive        ScheduleEntryStatus = "active"
	ScheduleEntryPausedManual  ScheduleEntryStatus = "paused_manual"
	ScheduleEntryPausedInvalid ScheduleEntryStatus = "paused_invalid"
)

type ScheduleEntryState struct {
	Plugin           string
	ScheduleID       string
	Command          string
	Status           ScheduleEntryStatus
	Reason           *string
	LastSuccessJobID *string
	LastSuccessAt    *time.Time
	NextRunAt        *time.Time
	UpdatedAt        time.Time
}

var ErrJobNotFound = errors.New("job not found")
var ErrDedupeDrop = errors.New("job dedupe drop")

type DedupeDropError struct {
	DedupeKey     string
	ExistingJobID string
}

func (e *DedupeDropError) Error() string {
	return fmt.Sprintf("job deduplicated for key %q (existing job %s)", e.DedupeKey, e.ExistingJobID)
}

func (e *DedupeDropError) Is(target error) bool {
	return target == ErrDedupeDrop
}

// JobResult is a lightweight projection for API job retrieval.
type JobResult struct {
	JobID       string
	Status      Status
	Plugin      string
	Command     string
	StepID      string // From event_context.step_id via event_context_id
	Result      json.RawMessage
	LastError   *string
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// JobSummary is a lightweight projection for jobs list APIs.
type JobSummary struct {
	JobID       string
	Plugin      string
	Command     string
	Status      Status
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Attempt     int
}
