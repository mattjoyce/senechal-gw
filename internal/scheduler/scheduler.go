package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/queue"
)

// Scheduler manages the scheduling and recovery of plugin jobs.
type Scheduler struct {
	cfg             *config.Config
	queue           QueueService // Use the interface here
	events          *events.Hub
	logger          *slog.Logger
	stopCh          chan struct{}
	wg              sync.WaitGroup
	supportsCommand func(pluginName, commandName string) bool
}

const (
	pollCommand   = "poll"
	handleCommand = "handle"
)

// Option mutates scheduler behavior.
type Option func(*Scheduler)

// WithCommandSupportChecker validates plugin command support using the discovered registry.
func WithCommandSupportChecker(fn func(pluginName, commandName string) bool) Option {
	return func(s *Scheduler) {
		s.supportsCommand = fn
	}
}

// New creates a new Scheduler instance.
func New(cfg *config.Config, q QueueService, hub *events.Hub, logger *slog.Logger, opts ...Option) *Scheduler {
	if hub == nil {
		hub = events.NewHub(128)
	}
	s := &Scheduler{
		cfg:    cfg,
		queue:  q,
		events: hub,
		logger: logger.With("component", "scheduler"),
		stopCh: make(chan struct{}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
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

		schedules := pluginConf.NormalizedSchedules()
		if len(schedules) == 0 {
			continue
		}

		for _, schedule := range schedules {
			s.processSchedule(ctx, pluginName, pluginConf, schedule)
		}
	}

	// Prune completed job logs
	if s.cfg.Service.JobLogRetention > 0 {
		if err := s.queue.PruneJobLogs(ctx, s.cfg.Service.JobLogRetention); err != nil {
			s.logger.Error("Failed to prune job logs", "error", err)
		}
	}
}

func (s *Scheduler) processSchedule(ctx context.Context, pluginName string, pluginConf config.PluginConf, schedule config.ScheduleConfig) {
	scheduleID := strings.TrimSpace(schedule.ID)
	if scheduleID == "" {
		scheduleID = "default"
	}

	command := strings.TrimSpace(schedule.Command)
	if command == "" {
		command = pollCommand
	}

	s.logger.Debug(
		"Processing schedule entry",
		"plugin", pluginName,
		"schedule_id", scheduleID,
		"command", command,
		"schedule_every", schedule.Every,
		"jitter", schedule.Jitter,
	)

	entryState, err := s.currentScheduleEntryState(ctx, pluginName, scheduleID, command)
	if err != nil {
		s.logger.Error("Failed to load schedule entry state", "plugin", pluginName, "schedule_id", scheduleID, "error", err)
		return
	}
	if entryState.Status == queue.ScheduleEntryPausedManual {
		s.events.Publish("scheduler.skipped", map[string]any{
			"plugin":      pluginName,
			"schedule_id": scheduleID,
			"command":     command,
			"reason":      "schedule_paused_manual",
		})
		return
	}

	baseInterval, err := parseScheduleEvery(schedule.Every)
	if err != nil {
		reason := "invalid_schedule_interval"
		s.logger.Error("Invalid schedule interval for plugin", "plugin", pluginName, "schedule_id", scheduleID, "interval", schedule.Every, "error", err)
		if upsertErr := s.pauseScheduleInvalid(ctx, pluginName, scheduleID, command, reason); upsertErr != nil {
			s.logger.Error("Failed to persist invalid schedule state", "plugin", pluginName, "schedule_id", scheduleID, "error", upsertErr)
		}
		s.events.Publish("scheduler.skipped", map[string]any{
			"plugin":      pluginName,
			"schedule_id": scheduleID,
			"command":     command,
			"reason":      reason,
		})
		return
	}

	if command == handleCommand {
		reason := "scheduled_handle_disallowed"
		if upsertErr := s.pauseScheduleInvalid(ctx, pluginName, scheduleID, command, reason); upsertErr != nil {
			s.logger.Error("Failed to persist invalid schedule state", "plugin", pluginName, "schedule_id", scheduleID, "error", upsertErr)
		}
		s.events.Publish("scheduler.skipped", map[string]any{
			"plugin":      pluginName,
			"schedule_id": scheduleID,
			"command":     command,
			"reason":      reason,
		})
		s.logger.Error("Scheduled command is not allowed", "plugin", pluginName, "schedule_id", scheduleID, "command", command)
		return
	}

	if s.supportsCommand != nil && !s.supportsCommand(pluginName, command) {
		reason := "command_not_supported"
		if upsertErr := s.pauseScheduleInvalid(ctx, pluginName, scheduleID, command, reason); upsertErr != nil {
			s.logger.Error("Failed to persist invalid schedule state", "plugin", pluginName, "schedule_id", scheduleID, "error", upsertErr)
		}
		s.events.Publish("scheduler.skipped", map[string]any{
			"plugin":      pluginName,
			"schedule_id": scheduleID,
			"command":     command,
			"reason":      reason,
		})
		s.logger.Warn("Scheduled command not found in plugin manifest", "plugin", pluginName, "schedule_id", scheduleID, "command", command)
		return
	}

	if entryState.Status == queue.ScheduleEntryPausedInvalid {
		if err := s.activateScheduleEntry(ctx, pluginName, scheduleID, command); err != nil {
			s.logger.Error("Failed to reactivate schedule entry", "plugin", pluginName, "schedule_id", scheduleID, "error", err)
			return
		}
	}

	// Calculate jittered interval (though not directly used in simplified MVP enqueue logic)
	_ = calculateJitteredInterval(baseInterval, schedule.Jitter)

	if err := s.reconcileCircuitBreaker(ctx, pluginName, command, pluginConf); err != nil {
		s.logger.Error("Failed to reconcile circuit breaker state", "plugin", pluginName, "schedule_id", scheduleID, "command", command, "error", err)
		return
	}

	if allowed, reason, err := s.canSchedule(ctx, pluginName, command, pluginConf); err != nil {
		s.logger.Error("Failed scheduling checks", "plugin", pluginName, "schedule_id", scheduleID, "command", command, "error", err)
		return
	} else if !allowed {
		s.events.Publish("scheduler.skipped", map[string]any{
			"plugin":      pluginName,
			"schedule_id": scheduleID,
			"command":     command,
			"reason":      reason,
		})
		s.logger.Info("Skipped scheduled job", "plugin", pluginName, "schedule_id", scheduleID, "command", command, "reason", reason)
		return
	}

	if err := s.enqueueScheduledJob(ctx, pluginName, scheduleID, command, pluginConf, schedule.Payload); err != nil {
		s.logger.Error("Failed to enqueue scheduled job", "plugin", pluginName, "schedule_id", scheduleID, "command", command, "error", err)
	}
}

func (s *Scheduler) currentScheduleEntryState(ctx context.Context, pluginName, scheduleID, command string) (queue.ScheduleEntryState, error) {
	state, err := s.queue.GetScheduleEntryState(ctx, pluginName, scheduleID)
	if err != nil {
		return queue.ScheduleEntryState{}, err
	}
	if state == nil {
		return queue.ScheduleEntryState{
			Plugin:     pluginName,
			ScheduleID: scheduleID,
			Command:    command,
			Status:     queue.ScheduleEntryActive,
		}, nil
	}
	if state.Command == "" {
		state.Command = command
	}
	if state.Status == "" {
		state.Status = queue.ScheduleEntryActive
	}
	return *state, nil
}

func (s *Scheduler) pauseScheduleInvalid(ctx context.Context, pluginName, scheduleID, command, reason string) error {
	reasonCopy := reason
	return s.queue.UpsertScheduleEntryState(ctx, queue.ScheduleEntryState{
		Plugin:     pluginName,
		ScheduleID: scheduleID,
		Command:    command,
		Status:     queue.ScheduleEntryPausedInvalid,
		Reason:     &reasonCopy,
	})
}

func (s *Scheduler) activateScheduleEntry(ctx context.Context, pluginName, scheduleID, command string) error {
	return s.queue.UpsertScheduleEntryState(ctx, queue.ScheduleEntryState{
		Plugin:     pluginName,
		ScheduleID: scheduleID,
		Command:    command,
		Status:     queue.ScheduleEntryActive,
		Reason:     nil,
	})
}

// enqueueScheduledJob creates and enqueues a command job for a given schedule entry.
func (s *Scheduler) enqueueScheduledJob(ctx context.Context, pluginName, scheduleID, command string, pluginConf config.PluginConf, payload map[string]any) error {
	dedupeKey := fmt.Sprintf("%s:%s:%s", pluginName, command, scheduleID)

	rawPayload := []byte(`{}`)
	if len(payload) > 0 {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal schedule payload: %w", err)
		}
		rawPayload = encoded
	}

	req := queue.EnqueueRequest{
		Plugin:      pluginName,
		Command:     command,
		Payload:     rawPayload,
		SubmittedBy: s.cfg.Service.Name,
		DedupeKey:   &dedupeKey,
	}
	if pluginConf.Retry != nil {
		req.MaxAttempts = pluginConf.Retry.MaxAttempts
	}

	jobID, err := s.queue.Enqueue(ctx, req)
	if err != nil {
		var dedupeErr *queue.DedupeDropError
		if errors.As(err, &dedupeErr) {
			s.logger.Info(
				"Skipped enqueue due to dedupe hit",
				"plugin", pluginName,
				"command", command,
				"schedule_id", scheduleID,
				"dedupe_key", dedupeErr.DedupeKey,
				"existing_job_id", dedupeErr.ExistingJobID,
			)
			return nil
		}
		return fmt.Errorf("enqueue scheduled job for %s/%s: %w", pluginName, command, err)
	}
	s.events.Publish("scheduler.scheduled", map[string]any{
		"job_id":      jobID,
		"plugin":      pluginName,
		"command":     command,
		"schedule_id": scheduleID,
	})
	s.logger.Info("Enqueued scheduled job", "plugin", pluginName, "command", command, "schedule_id", scheduleID, "job_id", jobID, "dedupe_key", dedupeKey)
	return nil
}

func (s *Scheduler) reconcileCircuitBreaker(ctx context.Context, pluginName, command string, pluginConf config.PluginConf) error {
	cb, err := s.currentCircuitBreaker(ctx, pluginName, command)
	if err != nil {
		return err
	}

	latest, err := s.queue.LatestCompletedCommandResult(ctx, pluginName, command, s.cfg.Service.Name)
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
			"command":        command,
			"previous_state": previousState,
			"state":          cb.State,
			"failure_count":  cb.FailureCount,
		})
		s.logger.Info(
			"Circuit breaker state changed",
			"plugin", pluginName,
			"command", command,
			"previous_state", previousState,
			"state", cb.State,
			"failure_count", cb.FailureCount,
		)
	}

	return nil
}

func (s *Scheduler) canSchedule(ctx context.Context, pluginName, command string, pluginConf config.PluginConf) (bool, string, error) {
	cb, err := s.currentCircuitBreaker(ctx, pluginName, command)
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
			"command": command,
		})
		s.logger.Info("Circuit breaker moved to half-open", "plugin", pluginName, "command", command)
	}

	maxOutstanding := pluginConf.MaxOutstandingPolls
	if maxOutstanding <= 0 {
		maxOutstanding = 1
	}

	outstanding, err := s.queue.CountOutstandingJobs(ctx, pluginName, command)
	if err != nil {
		return false, "", err
	}
	if outstanding >= maxOutstanding {
		return false, "outstanding_limit", nil
	}

	return true, "", nil
}

func (s *Scheduler) currentCircuitBreaker(ctx context.Context, pluginName, command string) (queue.CircuitBreaker, error) {
	cb, err := s.queue.GetCircuitBreaker(ctx, pluginName, command)
	if err != nil {
		return queue.CircuitBreaker{}, err
	}
	if cb == nil {
		return queue.CircuitBreaker{
			Plugin:       pluginName,
			Command:      command,
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
