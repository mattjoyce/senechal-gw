package queue

import (
	"encoding/json"
	"errors"
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

var ErrJobNotFound = errors.New("job not found")

// JobResult is a lightweight projection for API job retrieval.
type JobResult struct {
	JobID       string
	Status      Status
	Plugin      string
	Command     string
	Result      json.RawMessage
	LastError   *string
	StartedAt   *time.Time
	CompletedAt *time.Time
}
