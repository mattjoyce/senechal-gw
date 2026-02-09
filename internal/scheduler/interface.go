package scheduler

import (
	"context"
	"time"

	"github.com/mattjoyce/senechal-gw/internal/queue"
)

//go:generate mockgen -destination=mocks/mock_queue.go -package=mocks github.com/mattjoyce/senechal-gw/internal/scheduler QueueService

// QueueService defines the interface for queue operations used by the scheduler.
type QueueService interface {
	Enqueue(ctx context.Context, req queue.EnqueueRequest) (string, error)
	FindJobsByStatus(ctx context.Context, status queue.Status) ([]*queue.Job, error)
	UpdateJobForRecovery(ctx context.Context, jobID string, newStatus queue.Status, newAttempt int, nextRetryAt *time.Time, lastError string) error
	PruneJobLogs(ctx context.Context, retention time.Duration) error
}
