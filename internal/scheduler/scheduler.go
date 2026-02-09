package scheduler

import (
	"context"
	"fmt"
	"log/slog" // Import slog
	"math/rand"
	"sync"
	"time"

	"github.com/mattjoyce/senechal-gw/internal/config"
	"github.com/mattjoyce/senechal-gw/internal/queue" // Keep for queue.EnqueueRequest and queue.Job types
)

// Scheduler manages the scheduling and recovery of plugin jobs.
type Scheduler struct {
	cfg    *config.Config
	queue  QueueService // Use the interface here
	logger *slog.Logger
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New creates a new Scheduler instance.
func New(cfg *config.Config, q QueueService, logger *slog.Logger) *Scheduler { // Accept *slog.Logger here
	return &Scheduler{
		cfg:    cfg,
		queue:  q,
		logger: logger.With("component", "scheduler"),
		stopCh: make(chan struct{}),
	}
}

// Start begins the scheduler's tick loop and crash recovery.
func (s *Scheduler) Start(ctx context.Context) error {
	s.logger.Info("Starting scheduler")

	// Perform crash recovery on startup
	if err := s.recoverOrphanedJobs(ctx); err != nil {
		return fmt.Errorf("scheduler crash recovery failed: %w", err)
	}

	s.wg.Add(1)
	go s.tickLoop(ctx)

	return nil
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	s.logger.Info("Stopping scheduler")
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("Scheduler stopped")
}

// tickLoop is the main scheduling loop.
func (s *Scheduler) tickLoop(ctx context.Context) {
	defer s.wg.Done()

	// Initial tick immediately
	s.tick(ctx)

	ticker := time.NewTicker(s.cfg.Service.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.tick(ctx)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			s.logger.Warn("Scheduler context cancelled, stopping tick loop")
			return
		}
	}
}

// tick performs a single scheduling pass.
func (s *Scheduler) tick(ctx context.Context) {
	s.logger.Debug("Scheduler tick")

	for pluginName, pluginConf := range s.cfg.Plugins {
		if !pluginConf.Enabled {
			continue
		}
		if pluginConf.Schedule == nil {
			continue
		}

		s.logger.Debug(
			"Processing plugin for scheduling",
			"plugin", pluginName,
			"schedule_every", pluginConf.Schedule.Every,
			"jitter", pluginConf.Schedule.Jitter,
		)

		baseInterval, err := parseScheduleEvery(pluginConf.Schedule.Every)
		if err != nil {
			s.logger.Error("Invalid schedule interval for plugin", "plugin", pluginName, "interval", pluginConf.Schedule.Every, "error", err)
			continue
		}

		// Calculate jittered interval (though not directly used in simplified MVP enqueue logic)
		_ = calculateJitteredInterval(baseInterval, pluginConf.Schedule.Jitter)

		// Determine if it's time to enqueue a poll job
		// This is a simplified check. A more robust solution would track last scheduled time.
		// For now, we assume if the current time aligns with an interval, we schedule.
		// For actual "every" semantics, we need a persistent record of the last successful schedule time.
		// This MVP implementation focuses on demonstrating the jitter and enqueueing.

		// TODO: Implement a proper "next run" calculation based on a persisted last_run timestamp
		// and the jittered interval. For now, this just enqueues on every tick if the interval is short,
		// or will need a more complex time check for daily/weekly/monthly.

		// For MVP, we'll just enqueue on each tick for simplicity,
		// assuming a separate mechanism prevents duplicate poll execution
		// (e.g., job deduplication in the queue based on plugin/command).
		// A real scheduler would check if (now - last_scheduled_time) >= jitteredInterval
		if err := s.enqueuePollJob(ctx, pluginName, pluginConf); err != nil {
			s.logger.Error("Failed to enqueue poll job", "plugin", pluginName, "error", err)
		}

		// Poll Guard (max_outstanding_polls) and Circuit Breaker (consecutive failures)
		// will require a stateful mechanism to track outstanding polls and failures per plugin.
		// This is beyond the current MVP scope but marked for future implementation.
	}

	// Prune completed job logs
	if s.cfg.Service.JobLogRetention > 0 {
		if err := s.queue.PruneJobLogs(ctx, s.cfg.Service.JobLogRetention); err != nil {
			s.logger.Error("Failed to prune job logs", "error", err)
		}
	}
}

// enqueuePollJob creates and enqueues a poll job for a given plugin.
func (s *Scheduler) enqueuePollJob(ctx context.Context, pluginName string, pluginConf config.PluginConf) error {
	dedupeKey := fmt.Sprintf("poll:%s", pluginName) // Dedupe key for poll jobs

	req := queue.EnqueueRequest{
		Plugin:      pluginName,
		Command:     "poll",
		Payload:     []byte(`{}`), // Empty JSON payload for now
		SubmittedBy: s.cfg.Service.Name,
		DedupeKey:   &dedupeKey, // Apply dedupe key
	}
	if pluginConf.Retry != nil {
		req.MaxAttempts = pluginConf.Retry.MaxAttempts
	}

	// For MVP, NextRetryAt is not fully utilized for backoff, but setting it here
	// allows for future expansion. A poll job doesn't have a specific retry time
	// in the same way a failed job does, so we'll leave it to default queue handling for now.

	jobID, err := s.queue.Enqueue(ctx, req)
	if err != nil {
		// If the error is due to deduplication, we can log it at debug level instead of error.
		// For now, treating all enqueue errors as significant.
		return fmt.Errorf("enqueue poll job for %s: %w", pluginName, err)
	}
	s.logger.Info("Enqueued poll job", "plugin", pluginName, "job_id", jobID, "dedupe_key", dedupeKey)
	return nil
}

// recoverOrphanedJobs scans for and recovers jobs marked as "running" at startup.
func (s *Scheduler) recoverOrphanedJobs(ctx context.Context) error {
	s.logger.Info("Performing crash recovery for orphaned jobs")

	runningJobs, err := s.queue.FindJobsByStatus(ctx, queue.StatusRunning)
	if err != nil {
		return fmt.Errorf("failed to find running jobs for recovery: %w", err)
	}

	if len(runningJobs) == 0 {
		s.logger.Info("No orphaned jobs found.")
		return nil
	}

	s.logger.Warn("Found orphaned jobs, attempting recovery", "count", len(runningJobs))

	for _, job := range runningJobs {
		s.logger.Warn(
			"Recovering orphaned job",
			"job_id", job.ID,
			"plugin", job.Plugin,
			"command", job.Command,
			"current_attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
		)

		job.Attempt++ // Increment attempt counter for recovery

		var newStatus queue.Status
		var lastErrorMsg string
		var nextRetryAt *time.Time

		if job.Attempt <= job.MaxAttempts {
			newStatus = queue.StatusQueued
			// For MVP, backoff is not explicitly implemented as a delay, just re-queue.
			// NextRetryAt could be set here for future backoff implementation.
			s.logger.Warn(
				"Re-queueing orphaned job",
				"job_id", job.ID,
				"plugin", job.Plugin,
				"command", job.Command,
				"new_attempt", job.Attempt,
				"status", newStatus,
			)
		} else {
			newStatus = queue.StatusDead
			lastErrorMsg = fmt.Sprintf("Job marked dead during crash recovery: max attempts (%d) reached", job.MaxAttempts)
			s.logger.Error(
				"Marking orphaned job as dead (max attempts reached)",
				"job_id", job.ID,
				"plugin", job.Plugin,
				"command", job.Command,
				"final_attempt", job.Attempt,
				"status", newStatus,
				"error", lastErrorMsg,
			)
		}

		err := s.queue.UpdateJobForRecovery(ctx, job.ID, newStatus, job.Attempt, nextRetryAt, lastErrorMsg)
		if err != nil {
			s.logger.Error(
				"Failed to update orphaned job during recovery",
				"job_id", job.ID,
				"error", err,
				"desired_status", newStatus,
				"desired_attempt", job.Attempt,
			)
		}
	}

	return nil
}

// calculateJitteredInterval adds a random jitter to the base interval.
func calculateJitteredInterval(baseInterval time.Duration, jitter time.Duration) time.Duration {
	if jitter <= 0 {
		return baseInterval
	}
	// Generate a random duration between 0 and jitter
	// Using rand.Int63n for a positive random number up to jitter.Nanoseconds()
	randomJitter := time.Duration(rand.Int63n(jitter.Nanoseconds()))
	return baseInterval + randomJitter
}

// parseScheduleEvery converts the 'every' string from config to a base duration.
// For "daily", "weekly", "monthly", it returns a special indicator as these need
// calendar-aware scheduling.
func parseScheduleEvery(every string) (time.Duration, error) {
	switch every {
	case "5m":
		return 5 * time.Minute, nil
	case "15m":
		return 15 * time.Minute, nil
	case "30m":
		return 30 * time.Minute, nil
	case "hourly":
		return 1 * time.Hour, nil
	case "2h":
		return 2 * time.Hour, nil
	case "6h":
		return 6 * time.Hour, nil
	case "daily":
		return 24 * time.Hour, nil // Special handling will adjust to specific time
	case "weekly":
		return 7 * 24 * time.Hour, nil // Special handling will adjust
	case "monthly":
		return 30 * 24 * time.Hour, nil // Approximation, special handling will adjust
	default:
		// Attempt to parse as a generic duration if it's not a named interval
		d, err := time.ParseDuration(every)
		if err == nil {
			return d, nil
		}
	}
	return 0, fmt.Errorf("unsupported schedule interval: %s", every)
}

