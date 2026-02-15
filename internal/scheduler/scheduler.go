package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/queue"
)

// Scheduler manages the scheduling and recovery of plugin jobs.
type Scheduler struct {
	cfg    *config.Config
	queue  QueueService // Use the interface here
	events *events.Hub
	logger *slog.Logger
	stopCh chan struct{}
	wg     sync.WaitGroup
}

const pollCommand = "poll"

// New creates a new Scheduler instance.
func New(cfg *config.Config, q QueueService, hub *events.Hub, logger *slog.Logger) *Scheduler { // Accept *slog.Logger here
	if hub == nil {
		hub = events.NewHub(128)
	}
	return &Scheduler{
		cfg:    cfg,
		queue:  q,
		events: hub,
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
	s.events.Publish("scheduler.tick", map[string]any{
		"at": time.Now().UTC(),
	})

	// Sort plugin names for deterministic iteration (critical for testing)
	var pluginNames []string
	for name := range s.cfg.Plugins {
		pluginNames = append(pluginNames, name)
	}
	sort.Strings(pluginNames)

	for _, pluginName := range pluginNames {
		pluginConf := s.cfg.Plugins[pluginName]
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
		// but guard poll fanout and breaker state before enqueueing.
		if err := s.reconcileCircuitBreaker(ctx, pluginName, pluginConf); err != nil {
			s.logger.Error("Failed to reconcile circuit breaker state", "plugin", pluginName, "error", err)
			continue
		}

		if allowed, reason, err := s.canSchedulePoll(ctx, pluginName, pluginConf); err != nil {
			s.logger.Error("Failed poll scheduling checks", "plugin", pluginName, "error", err)
			continue
		} else if !allowed {
			s.events.Publish("scheduler.skipped", map[string]any{
				"plugin":  pluginName,
				"command": pollCommand,
				"reason":  reason,
			})
			s.logger.Info("Skipped poll scheduling", "plugin", pluginName, "reason", reason)
			continue
		}

		if err := s.enqueuePollJob(ctx, pluginName, pluginConf); err != nil {
			s.logger.Error("Failed to enqueue poll job", "plugin", pluginName, "error", err)
		}
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
		Command:     pollCommand,
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
		var dedupeErr *queue.DedupeDropError
		if errors.As(err, &dedupeErr) {
			s.logger.Info(
				"Skipped poll enqueue due to dedupe hit",
				"plugin", pluginName,
				"dedupe_key", dedupeErr.DedupeKey,
				"existing_job_id", dedupeErr.ExistingJobID,
			)
			return nil
		}
		// If the error is due to deduplication, we can log it at debug level instead of error.
		// For now, treating all enqueue errors as significant.
		return fmt.Errorf("enqueue poll job for %s: %w", pluginName, err)
	}
	s.events.Publish("scheduler.scheduled", map[string]any{
		"job_id":  jobID,
		"plugin":  pluginName,
		"command": pollCommand,
	})
	s.logger.Info("Enqueued poll job", "plugin", pluginName, "job_id", jobID, "dedupe_key", dedupeKey)
	return nil
}

func (s *Scheduler) reconcileCircuitBreaker(ctx context.Context, pluginName string, pluginConf config.PluginConf) error {
	cb, err := s.currentCircuitBreaker(ctx, pluginName)
	if err != nil {
		return err
	}

	latest, err := s.queue.LatestCompletedPollResult(ctx, pluginName, s.cfg.Service.Name)
	if err != nil {
		return err
	}
	if latest == nil {
		return nil
	}
	if cb.LastJobID != nil && *cb.LastJobID == latest.JobID {
		return nil
	}

	threshold := breakerThreshold(pluginConf)
	previousState := cb.State

	if latest.Status == queue.StatusSucceeded {
		cb.State = queue.CircuitClosed
		cb.FailureCount = 0
		cb.OpenedAt = nil
		cb.LastFailure = nil
	} else {
		cb.FailureCount++
		completedAt := latest.CompletedAt.UTC()
		cb.LastFailure = &completedAt
		if cb.FailureCount >= threshold {
			cb.State = queue.CircuitOpen
			cb.OpenedAt = &completedAt
		} else {
			cb.State = queue.CircuitClosed
			cb.OpenedAt = nil
		}
	}

	cb.LastJobID = &latest.JobID
	if err := s.queue.UpsertCircuitBreaker(ctx, cb); err != nil {
		return err
	}

	if cb.State != previousState {
		s.events.Publish("scheduler.circuit_state_changed", map[string]any{
			"plugin":         pluginName,
			"command":        pollCommand,
			"previous_state": previousState,
			"state":          cb.State,
			"failure_count":  cb.FailureCount,
		})
		s.logger.Info(
			"Circuit breaker state changed",
			"plugin", pluginName,
			"command", pollCommand,
			"previous_state", previousState,
			"state", cb.State,
			"failure_count", cb.FailureCount,
		)
	}

	return nil
}

func (s *Scheduler) canSchedulePoll(ctx context.Context, pluginName string, pluginConf config.PluginConf) (bool, string, error) {
	cb, err := s.currentCircuitBreaker(ctx, pluginName)
	if err != nil {
		return false, "", err
	}

	if cb.State == queue.CircuitOpen {
		if !cooldownElapsed(cb, breakerResetAfter(pluginConf), time.Now().UTC()) {
			return false, "circuit_open", nil
		}

		cb.State = queue.CircuitHalfOpen
		if err := s.queue.UpsertCircuitBreaker(ctx, cb); err != nil {
			return false, "", err
		}
		s.events.Publish("scheduler.circuit_half_open", map[string]any{
			"plugin":  pluginName,
			"command": pollCommand,
		})
		s.logger.Info("Circuit breaker moved to half-open", "plugin", pluginName, "command", pollCommand)
	}

	maxOutstanding := pluginConf.MaxOutstandingPolls
	if maxOutstanding <= 0 {
		maxOutstanding = 1
	}

	outstanding, err := s.queue.CountOutstandingPollJobs(ctx, pluginName)
	if err != nil {
		return false, "", err
	}
	if outstanding >= maxOutstanding {
		return false, "poll_guard_outstanding", nil
	}

	return true, "", nil
}

func (s *Scheduler) currentCircuitBreaker(ctx context.Context, pluginName string) (queue.CircuitBreaker, error) {
	cb, err := s.queue.GetCircuitBreaker(ctx, pluginName, pollCommand)
	if err != nil {
		return queue.CircuitBreaker{}, err
	}
	if cb == nil {
		return queue.CircuitBreaker{
			Plugin:       pluginName,
			Command:      pollCommand,
			State:        queue.CircuitClosed,
			FailureCount: 0,
		}, nil
	}
	if cb.State == "" {
		cb.State = queue.CircuitClosed
	}
	return *cb, nil
}

func breakerThreshold(pluginConf config.PluginConf) int {
	if pluginConf.CircuitBreaker == nil || pluginConf.CircuitBreaker.Threshold <= 0 {
		return config.DefaultPluginConf().CircuitBreaker.Threshold
	}
	return pluginConf.CircuitBreaker.Threshold
}

func breakerResetAfter(pluginConf config.PluginConf) time.Duration {
	if pluginConf.CircuitBreaker == nil || pluginConf.CircuitBreaker.ResetAfter <= 0 {
		return config.DefaultPluginConf().CircuitBreaker.ResetAfter
	}
	return pluginConf.CircuitBreaker.ResetAfter
}

func cooldownElapsed(cb queue.CircuitBreaker, resetAfter time.Duration, now time.Time) bool {
	if cb.OpenedAt == nil {
		return true
	}
	return !now.Before(cb.OpenedAt.Add(resetAfter))
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
func parseScheduleEvery(every string) (time.Duration, error) {
	return config.ParseInterval(every)
}
