package scheduler

import (
	"context"
	"time"

	"github.com/mattjoyce/ductile/internal/queue"
)

//go:generate mockgen -destination=mocks/mock_queue.go -package=mocks github.com/mattjoyce/ductile/internal/scheduler QueueService

// QueueService defines the interface for queue operations used by the scheduler.
type QueueService interface {
	Enqueue(ctx context.Context, req queue.EnqueueRequest) (string, error)
	CountOutstandingJobs(ctx context.Context, plugin, command string) (int, error)
	LatestCompletedCommandResult(ctx context.Context, plugin, command, submittedBy string) (*queue.CommandResult, error)
	GetCircuitBreaker(ctx context.Context, plugin, command string) (*queue.CircuitBreaker, error)
	UpsertCircuitBreaker(ctx context.Context, cb queue.CircuitBreaker) error
	ResetCircuitBreaker(ctx context.Context, plugin, command string) error
	GetScheduleEntryState(ctx context.Context, plugin, scheduleID string) (*queue.ScheduleEntryState, error)
	UpsertScheduleEntryState(ctx context.Context, state queue.ScheduleEntryState) error
	FindJobsByStatus(ctx context.Context, status queue.Status) ([]*queue.Job, error)
	UpdateJobForRecovery(ctx context.Context, jobID string, newStatus queue.Status, newAttempt int, nextRetryAt *time.Time, lastError string) error
	PruneJobLogs(ctx context.Context, retention time.Duration) error
}
