package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/storage"
)

func setupSchedulerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "scheduler.db")
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRecoverOrphanedJobs_RealQueue(t *testing.T) {
	t.Run("requeues retryable job and kills exhausted job", func(t *testing.T) {
		db := setupSchedulerTestDB(t)
		q := queue.New(db)
		logger, logBuf := NewTestSlogger()
		s := New(config.Defaults(), q, events.NewHub(32), logger)
		ctx := context.Background()

		jobRetry, err := q.Enqueue(ctx, queue.EnqueueRequest{Plugin: "pluginA", Command: "poll", SubmittedBy: "test", MaxAttempts: 3})
		if err != nil {
			t.Fatalf("enqueue retry job: %v", err)
		}
		jobDead, err := q.Enqueue(ctx, queue.EnqueueRequest{Plugin: "pluginB", Command: "handle", SubmittedBy: "test", MaxAttempts: 3})
		if err != nil {
			t.Fatalf("enqueue dead job: %v", err)
		}

		j1, err := q.Dequeue(ctx)
		if err != nil || j1 == nil {
			t.Fatalf("dequeue first job: %v job=%v", err, j1)
		}
		j2, err := q.Dequeue(ctx)
		if err != nil || j2 == nil {
			t.Fatalf("dequeue second job: %v job=%v", err, j2)
		}
		if j1.ID == jobDead {
			j1, j2 = j2, j1
		}

		if err := q.UpdateJobForRecovery(ctx, j1.ID, queue.StatusRunning, 1, nil, ""); err != nil {
			t.Fatalf("set attempt 1: %v", err)
		}
		if err := q.UpdateJobForRecovery(ctx, j2.ID, queue.StatusRunning, 3, nil, ""); err != nil {
			t.Fatalf("set attempt 3: %v", err)
		}

		if err := s.recoverOrphanedJobs(ctx); err != nil {
			t.Fatalf("recoverOrphanedJobs: %v", err)
		}

		retried, err := q.GetJobByID(ctx, jobRetry)
		if err != nil {
			t.Fatalf("GetJobByID retry: %v", err)
		}
		if retried.Status != queue.StatusQueued {
			t.Fatalf("retry job status = %s, want queued", retried.Status)
		}

		dead, err := q.GetJobByID(ctx, jobDead)
		if err != nil {
			t.Fatalf("GetJobByID dead: %v", err)
		}
		if dead.Status != queue.StatusDead {
			t.Fatalf("dead job status = %s, want dead", dead.Status)
		}

		if got := logBuf.String(); got == "" || !strings.Contains(got, "Re-queueing orphaned job") || !strings.Contains(got, "Marking orphaned job as dead") {
			t.Fatalf("unexpected log output: %s", got)
		}
	})

	t.Run("returns wrapped lookup error", func(t *testing.T) {
		logger, _ := NewTestSlogger()
		s := New(config.Defaults(), failingRecoveryQueue{}, events.NewHub(32), logger)
		err := s.recoverOrphanedJobs(context.Background())
		if err == nil || !strings.Contains(err.Error(), "failed to find running jobs for recovery: db error") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

type failingRecoveryQueue struct{}

func (failingRecoveryQueue) Enqueue(context.Context, queue.EnqueueRequest) (string, error) {
	return "", errors.New("unexpected")
}
func (failingRecoveryQueue) CountOutstandingJobs(context.Context, string, string) (int, error) {
	return 0, errors.New("unexpected")
}
func (failingRecoveryQueue) CancelOutstandingJobs(context.Context, string, string, string) (int, error) {
	return 0, errors.New("unexpected")
}
func (failingRecoveryQueue) LatestCompletedCommandResult(context.Context, string, string, string) (*queue.CommandResult, error) {
	return nil, errors.New("unexpected")
}
func (failingRecoveryQueue) GetCircuitBreaker(context.Context, string, string) (*queue.CircuitBreaker, error) {
	return nil, errors.New("unexpected")
}
func (failingRecoveryQueue) UpsertCircuitBreaker(context.Context, queue.CircuitBreaker) error {
	return errors.New("unexpected")
}
func (failingRecoveryQueue) ResetCircuitBreaker(context.Context, string, string) error {
	return errors.New("unexpected")
}
func (failingRecoveryQueue) GetScheduleEntryState(context.Context, string, string) (*queue.ScheduleEntryState, error) {
	return nil, errors.New("unexpected")
}
func (failingRecoveryQueue) UpsertScheduleEntryState(context.Context, queue.ScheduleEntryState) error {
	return errors.New("unexpected")
}
func (failingRecoveryQueue) FindJobsByStatus(context.Context, queue.Status) ([]*queue.Job, error) {
	return nil, errors.New("db error")
}
func (failingRecoveryQueue) UpdateJobForRecovery(context.Context, string, queue.Status, int, *time.Time, string) error {
	return errors.New("unexpected")
}
func (failingRecoveryQueue) PruneJobLogs(context.Context, time.Duration) error { return nil }

func TestSchedulerTickEnqueuesWhenAllowed_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, logBuf := NewTestSlogger()

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {
			Enabled:             true,
			Schedules:           []config.ScheduleConfig{{Every: "1m"}},
			Retry:               &config.RetryConfig{MaxAttempts: 3},
			CircuitBreaker:      &config.CircuitBreakerConfig{Threshold: 3, ResetAfter: 30 * time.Minute},
			MaxOutstandingPolls: 1,
		},
	}

	s := New(cfg, q, events.NewHub(64), logger)
	s.tick(context.Background())

	jobs, total, err := q.ListJobs(context.Background(), queue.ListJobsFilter{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 1 || len(jobs) != 1 {
		t.Fatalf("expected 1 job, got total=%d len=%d", total, len(jobs))
	}
	if jobs[0].Plugin != "echo" || jobs[0].Command != pollCommand || jobs[0].Status != queue.StatusQueued {
		t.Fatalf("unexpected job summary: %+v", jobs[0])
	}

	state, err := q.GetScheduleEntryState(context.Background(), "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState: %v", err)
	}
	if state == nil || state.LastFiredAt == nil || state.Command != pollCommand {
		t.Fatalf("unexpected schedule state: %+v", state)
	}
	if !strings.Contains(logBuf.String(), "Enqueued scheduled job") {
		t.Fatalf("expected enqueue log, got: %s", logBuf.String())
	}
}

func TestSchedulerTickSkipsWhenNotDueAfterSuccess_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, logBuf := NewTestSlogger()
	ctx := context.Background()

	completedJobID, err := q.Enqueue(ctx, queue.EnqueueRequest{Plugin: "echo", Command: pollCommand, SubmittedBy: "ductile"})
	if err != nil {
		t.Fatalf("enqueue completed job: %v", err)
	}
	if _, err := q.Dequeue(ctx); err != nil {
		t.Fatalf("dequeue completed job: %v", err)
	}
	if err := q.CompleteWithResult(ctx, completedJobID, queue.StatusSucceeded, nil, nil, nil); err != nil {
		t.Fatalf("complete completed job: %v", err)
	}

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {Enabled: true, Schedules: []config.ScheduleConfig{{Every: "1h"}}},
	}

	s := New(cfg, q, events.NewHub(64), logger)
	s.tick(ctx)

	jobs, total, err := q.ListJobs(ctx, queue.ListJobsFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 1 || len(jobs) != 1 {
		t.Fatalf("expected no new jobs, got total=%d len=%d", total, len(jobs))
	}
	state, err := q.GetScheduleEntryState(ctx, "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState: %v", err)
	}
	if state == nil || state.LastSuccessJobID == nil || *state.LastSuccessJobID != completedJobID || state.LastSuccessAt == nil || state.NextRunAt == nil {
		t.Fatalf("unexpected schedule state: %+v", state)
	}
	if !state.NextRunAt.After(*state.LastSuccessAt) {
		t.Fatalf("expected next_run_at after last success, got state=%+v", state)
	}
	if !strings.Contains(logBuf.String(), "Skipped scheduled job (not due)") {
		t.Fatalf("expected not-due log, got: %s", logBuf.String())
	}
}

func TestSchedulerTickDedupeHitAdvancesNextRun_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, logBuf := NewTestSlogger()
	ctx := context.Background()

	dedupeKey := "echo:poll:default"
	existingJobID, err := q.Enqueue(ctx, queue.EnqueueRequest{
		Plugin:      "echo",
		Command:     pollCommand,
		SubmittedBy: "previous",
		DedupeKey:   &dedupeKey,
		DedupeTTL:   durationPtr(time.Minute),
	})
	if err != nil {
		t.Fatalf("enqueue existing dedupe job: %v", err)
	}
	if _, err := q.Dequeue(ctx); err != nil {
		t.Fatalf("dequeue existing dedupe job: %v", err)
	}
	if err := q.CompleteWithResult(ctx, existingJobID, queue.StatusSucceeded, nil, nil, nil); err != nil {
		t.Fatalf("complete existing dedupe job: %v", err)
	}

	pastDue := time.Now().UTC().Add(-30 * time.Second)
	if err := q.UpsertScheduleEntryState(ctx, queue.ScheduleEntryState{
		Plugin:     "echo",
		ScheduleID: "default",
		Command:    pollCommand,
		Status:     queue.ScheduleEntryActive,
		NextRunAt:  &pastDue,
	}); err != nil {
		t.Fatalf("UpsertScheduleEntryState: %v", err)
	}

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {Enabled: true, Schedules: []config.ScheduleConfig{{Every: "1m"}}},
	}

	s := New(cfg, q, events.NewHub(64), logger)
	callStart := time.Now().UTC()
	s.tick(ctx)

	jobs, total, err := q.ListJobs(ctx, queue.ListJobsFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 1 || len(jobs) != 1 {
		t.Fatalf("expected no additional jobs after dedupe hit, got total=%d len=%d", total, len(jobs))
	}
	state, err := q.GetScheduleEntryState(ctx, "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState: %v", err)
	}
	if state == nil || state.NextRunAt == nil {
		t.Fatalf("expected updated schedule state, got %+v", state)
	}
	if state.NextRunAt.Before(callStart.Add(57*time.Second)) || state.NextRunAt.After(callStart.Add(63*time.Second)) {
		t.Fatalf("expected next_run_at about 1m ahead, got %v", *state.NextRunAt)
	}
	if !strings.Contains(logBuf.String(), "Skipped enqueue due to dedupe hit") {
		t.Fatalf("expected dedupe log, got: %s", logBuf.String())
	}
}

func TestSchedulerTickCronNotDueWithFutureNextRun_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, logBuf := NewTestSlogger()
	ctx := context.Background()

	nextRunAt := time.Now().UTC().Add(2 * time.Minute)
	if err := q.UpsertScheduleEntryState(ctx, queue.ScheduleEntryState{
		Plugin:     "echo",
		ScheduleID: "default",
		Command:    pollCommand,
		Status:     queue.ScheduleEntryActive,
		NextRunAt:  &nextRunAt,
	}); err != nil {
		t.Fatalf("UpsertScheduleEntryState: %v", err)
	}

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {Enabled: true, Schedules: []config.ScheduleConfig{{Cron: "* * * * *"}}},
	}

	s := New(cfg, q, events.NewHub(64), logger)
	s.tick(ctx)

	jobs, total, err := q.ListJobs(ctx, queue.ListJobsFilter{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 0 || len(jobs) != 0 {
		t.Fatalf("expected no cron jobs while not due, got total=%d len=%d", total, len(jobs))
	}
	if !strings.Contains(logBuf.String(), "Skipped scheduled job (not due)") {
		t.Fatalf("expected not-due log, got: %s", logBuf.String())
	}
}

func TestSchedulerTickPollGuardSkipsEnqueue_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, logBuf := NewTestSlogger()
	ctx := context.Background()

	_, err := q.Enqueue(ctx, queue.EnqueueRequest{Plugin: "echo", Command: pollCommand, SubmittedBy: "existing"})
	if err != nil {
		t.Fatalf("enqueue existing job: %v", err)
	}

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {
			Enabled:             true,
			Schedules:           []config.ScheduleConfig{{Every: "1m"}},
			MaxOutstandingPolls: 1,
			CircuitBreaker:      &config.CircuitBreakerConfig{Threshold: 3, ResetAfter: 30 * time.Minute},
		},
	}

	s := New(cfg, q, events.NewHub(64), logger)
	s.tick(ctx)

	jobs, total, err := q.ListJobs(ctx, queue.ListJobsFilter{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 1 || len(jobs) != 1 {
		t.Fatalf("expected still 1 job, got total=%d len=%d", total, len(jobs))
	}
	if !strings.Contains(logBuf.String(), "Skipped scheduled job") {
		t.Fatalf("expected skip log, got: %s", logBuf.String())
	}
}

func TestSchedulerTickRejectsScheduledHandle_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, logBuf := NewTestSlogger()
	ctx := context.Background()

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {
			Enabled:   true,
			Schedules: []config.ScheduleConfig{{Every: "1m", Command: handleCommand}},
		},
	}

	s := New(cfg, q, events.NewHub(64), logger)
	s.tick(ctx)

	state, err := q.GetScheduleEntryState(ctx, "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState: %v", err)
	}
	if state == nil {
		t.Fatal("expected persisted schedule state")
	}
	if state.Status != queue.ScheduleEntryPausedInvalid || state.Reason == nil || *state.Reason != "scheduled_handle_disallowed" {
		t.Fatalf("unexpected state: %+v", state)
	}
	jobs, total, err := q.ListJobs(ctx, queue.ListJobsFilter{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 0 || len(jobs) != 0 {
		t.Fatalf("expected no jobs, got total=%d len=%d", total, len(jobs))
	}
	if !strings.Contains(logBuf.String(), "Scheduled command is not allowed") {
		t.Fatalf("expected invalid command log, got: %s", logBuf.String())
	}
}

func TestSchedulerTickCronDueEnqueuesAndAdvancesNextRun_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, _ := NewTestSlogger()
	ctx := context.Background()

	now := time.Now().UTC()
	cronExpr := fmt.Sprintf("%d * * * *", now.Minute())

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.TickInterval = 30 * time.Second
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {
			Enabled:             true,
			Schedules:           []config.ScheduleConfig{{Cron: cronExpr}},
			CircuitBreaker:      &config.CircuitBreakerConfig{Threshold: 3, ResetAfter: 30 * time.Minute},
			MaxOutstandingPolls: 1,
		},
	}

	s := New(cfg, q, events.NewHub(64), logger)
	s.tick(ctx)

	jobs, total, err := q.ListJobs(ctx, queue.ListJobsFilter{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 1 || len(jobs) != 1 {
		t.Fatalf("expected 1 cron job, got total=%d len=%d", total, len(jobs))
	}

	state, err := q.GetScheduleEntryState(ctx, "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState: %v", err)
	}
	if state == nil || state.NextRunAt == nil {
		t.Fatalf("expected next run state, got %+v", state)
	}
	if !state.NextRunAt.After(now.Add(-1 * time.Minute)) {
		t.Fatalf("expected future-ish next_run_at, got %v", *state.NextRunAt)
	}
}

func TestCatchUpScheduleRunOnce_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, _ := NewTestSlogger()
	ctx := context.Background()

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	s := New(cfg, q, events.NewHub(64), logger)

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	lastFired := now.Add(-20 * time.Minute)
	if err := q.UpsertScheduleEntryState(ctx, queue.ScheduleEntryState{
		Plugin:      "echo",
		ScheduleID:  "default",
		Command:     pollCommand,
		Status:      queue.ScheduleEntryActive,
		LastFiredAt: &lastFired,
	}); err != nil {
		t.Fatalf("UpsertScheduleEntryState: %v", err)
	}

	err := s.catchUpSchedule(ctx, "echo", config.PluginConf{Enabled: true}, config.ScheduleConfig{ID: "default", Every: "5m", CatchUp: "run_once"}, now)
	if err != nil {
		t.Fatalf("catchUpSchedule: %v", err)
	}

	jobs, total, err := q.ListJobs(ctx, queue.ListJobsFilter{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 1 || len(jobs) != 1 {
		t.Fatalf("expected 1 catch-up job, got total=%d len=%d", total, len(jobs))
	}

	state, err := q.GetScheduleEntryState(ctx, "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState: %v", err)
	}
	if state == nil || state.LastFiredAt == nil || !state.LastFiredAt.After(lastFired) || state.NextRunAt == nil {
		t.Fatalf("unexpected schedule state: %+v", state)
	}
}

func TestCatchUpScheduleRunAllBounded_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, _ := NewTestSlogger()
	ctx := context.Background()

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	s := New(cfg, q, events.NewHub(64), logger)

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	lastFired := now.Add(-1000 * time.Minute)
	if err := q.UpsertScheduleEntryState(ctx, queue.ScheduleEntryState{
		Plugin:      "echo",
		ScheduleID:  "default",
		Command:     pollCommand,
		Status:      queue.ScheduleEntryActive,
		LastFiredAt: &lastFired,
	}); err != nil {
		t.Fatalf("UpsertScheduleEntryState: %v", err)
	}

	err := s.catchUpSchedule(ctx, "echo", config.PluginConf{Enabled: true}, config.ScheduleConfig{ID: "default", Every: "5m", CatchUp: "run_all"}, now)
	if err != nil {
		t.Fatalf("catchUpSchedule: %v", err)
	}

	jobs, total, err := q.ListJobs(ctx, queue.ListJobsFilter{Limit: catchUpRunAllMax})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != catchUpRunAllMax || len(jobs) != catchUpRunAllMax {
		t.Fatalf("expected %d catch-up jobs, got total=%d len=%d", catchUpRunAllMax, total, len(jobs))
	}

	state, err := q.GetScheduleEntryState(ctx, "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState: %v", err)
	}
	expected := lastFired.Add(5 * time.Minute * time.Duration(catchUpRunAllMax))
	if state == nil || state.LastFiredAt == nil || state.NextRunAt == nil {
		t.Fatalf("unexpected state: %+v", state)
	}
	if state.LastFiredAt.Sub(expected) > time.Second || expected.Sub(*state.LastFiredAt) > time.Second {
		t.Fatalf("LastFiredAt = %v, want near %v", *state.LastFiredAt, expected)
	}
}

func TestSchedulerTickOneShotAfterNotDue_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, logBuf := NewTestSlogger()
	ctx := context.Background()

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {Enabled: true, Schedules: []config.ScheduleConfig{{After: 2 * time.Hour}}},
	}

	s := New(cfg, q, events.NewHub(64), logger)
	s.tick(ctx)

	jobs, total, err := q.ListJobs(ctx, queue.ListJobsFilter{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 0 || len(jobs) != 0 {
		t.Fatalf("expected no one-shot jobs yet, got total=%d len=%d", total, len(jobs))
	}
	state, err := q.GetScheduleEntryState(ctx, "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState: %v", err)
	}
	if state == nil || state.Status != queue.ScheduleEntryActive || state.NextRunAt == nil {
		t.Fatalf("unexpected state: %+v", state)
	}
	if !strings.Contains(logBuf.String(), "Skipped scheduled job (not due)") {
		t.Fatalf("expected not-due log, got: %s", logBuf.String())
	}
}

func TestSchedulerTickOneShotAtDueExhausts_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, _ := NewTestSlogger()
	ctx := context.Background()

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {
			Enabled:             true,
			Schedules:           []config.ScheduleConfig{{At: "2026-03-01T00:00:00Z"}},
			CircuitBreaker:      &config.CircuitBreakerConfig{Threshold: 3, ResetAfter: 30 * time.Minute},
			MaxOutstandingPolls: 1,
		},
	}

	s := New(cfg, q, events.NewHub(64), logger)
	s.tick(ctx)

	jobs, total, err := q.ListJobs(ctx, queue.ListJobsFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 1 || len(jobs) != 1 {
		t.Fatalf("expected one one-shot job, got total=%d len=%d", total, len(jobs))
	}
	state, err := q.GetScheduleEntryState(ctx, "echo", "default")
	if err != nil {
		t.Fatalf("GetScheduleEntryState: %v", err)
	}
	if state == nil || state.Status != queue.ScheduleEntryExhausted || state.Reason == nil || *state.Reason != "one_shot_exhausted" || state.NextRunAt != nil {
		t.Fatalf("unexpected exhausted state: %+v", state)
	}
}

func TestCircuitBreakerStateTransitions_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, _ := NewTestSlogger()
	ctx := context.Background()

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	s := New(cfg, q, events.NewHub(64), logger)
	pluginConf := config.PluginConf{
		CircuitBreaker:      &config.CircuitBreakerConfig{Threshold: 2, ResetAfter: 30 * time.Minute},
		MaxOutstandingPolls: 1,
	}

	job1, err := q.Enqueue(ctx, queue.EnqueueRequest{Plugin: "echo", Command: pollCommand, SubmittedBy: "ductile"})
	if err != nil {
		t.Fatalf("enqueue job1: %v", err)
	}
	if _, err := q.Dequeue(ctx); err != nil {
		t.Fatalf("dequeue job1: %v", err)
	}
	if err := q.CompleteWithResult(ctx, job1, queue.StatusFailed, nil, nil, nil); err != nil {
		t.Fatalf("complete job1: %v", err)
	}

	if _, err := s.reconcileCircuitBreaker(ctx, "echo", pollCommand, pluginConf); err != nil {
		t.Fatalf("reconcile first failure: %v", err)
	}
	cb, err := q.GetCircuitBreaker(ctx, "echo", pollCommand)
	if err != nil {
		t.Fatalf("GetCircuitBreaker first: %v", err)
	}
	if cb == nil || cb.State != queue.CircuitClosed || cb.FailureCount != 1 || cb.LastJobID == nil || *cb.LastJobID != job1 {
		t.Fatalf("unexpected cb after first failure: %+v", cb)
	}

	job2, err := q.Enqueue(ctx, queue.EnqueueRequest{Plugin: "echo", Command: pollCommand, SubmittedBy: "ductile"})
	if err != nil {
		t.Fatalf("enqueue job2: %v", err)
	}
	if _, err := q.Dequeue(ctx); err != nil {
		t.Fatalf("dequeue job2: %v", err)
	}
	if err := q.CompleteWithResult(ctx, job2, queue.StatusFailed, nil, nil, nil); err != nil {
		t.Fatalf("complete job2: %v", err)
	}

	if _, err := s.reconcileCircuitBreaker(ctx, "echo", pollCommand, pluginConf); err != nil {
		t.Fatalf("reconcile second failure: %v", err)
	}
	cb, err = q.GetCircuitBreaker(ctx, "echo", pollCommand)
	if err != nil {
		t.Fatalf("GetCircuitBreaker second: %v", err)
	}
	if cb == nil || cb.State != queue.CircuitOpen || cb.FailureCount != 2 || cb.OpenedAt == nil {
		t.Fatalf("unexpected cb after second failure: %+v", cb)
	}

	allowed, reason, err := s.canSchedule(ctx, "echo", pollCommand, pluginConf, config.ScheduleConfig{})
	if err != nil {
		t.Fatalf("canSchedule open: %v", err)
	}
	if allowed || reason != "circuit_open" {
		t.Fatalf("expected circuit_open, got allowed=%v reason=%q", allowed, reason)
	}

	openedAtElapsed := time.Now().UTC().Add(-31 * time.Minute)
	if err := q.UpsertCircuitBreaker(ctx, queue.CircuitBreaker{Plugin: "echo", Command: pollCommand, State: queue.CircuitOpen, FailureCount: 2, OpenedAt: &openedAtElapsed}); err != nil {
		t.Fatalf("UpsertCircuitBreaker half-open seed: %v", err)
	}
	allowed, reason, err = s.canSchedule(ctx, "echo", pollCommand, pluginConf, config.ScheduleConfig{})
	if err != nil {
		t.Fatalf("canSchedule half-open transition: %v", err)
	}
	if !allowed || reason != "" {
		t.Fatalf("expected scheduling allowed after reset window, got allowed=%v reason=%q", allowed, reason)
	}
	cb, err = q.GetCircuitBreaker(ctx, "echo", pollCommand)
	if err != nil {
		t.Fatalf("GetCircuitBreaker half-open: %v", err)
	}
	if cb == nil || cb.State != queue.CircuitHalfOpen {
		t.Fatalf("expected half-open breaker, got %+v", cb)
	}

	job3, err := q.Enqueue(ctx, queue.EnqueueRequest{Plugin: "echo", Command: pollCommand, SubmittedBy: "ductile"})
	if err != nil {
		t.Fatalf("enqueue job3: %v", err)
	}
	if _, err := q.Dequeue(ctx); err != nil {
		t.Fatalf("dequeue job3: %v", err)
	}
	if err := q.CompleteWithResult(ctx, job3, queue.StatusSucceeded, nil, nil, nil); err != nil {
		t.Fatalf("complete job3: %v", err)
	}
	if _, err := s.reconcileCircuitBreaker(ctx, "echo", pollCommand, pluginConf); err != nil {
		t.Fatalf("reconcile success: %v", err)
	}
	cb, err = q.GetCircuitBreaker(ctx, "echo", pollCommand)
	if err != nil {
		t.Fatalf("GetCircuitBreaker final: %v", err)
	}
	if cb == nil || cb.State != queue.CircuitClosed || cb.FailureCount != 0 || cb.OpenedAt != nil {
		t.Fatalf("unexpected final cb: %+v", cb)
	}
}

func TestCanScheduleIfRunningPolicies_RealQueue(t *testing.T) {
	db := setupSchedulerTestDB(t)
	q := queue.New(db)
	logger, _ := NewTestSlogger()
	ctx := context.Background()

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	s := New(cfg, q, events.NewHub(64), logger)
	pluginConf := config.PluginConf{
		MaxOutstandingPolls: 1,
		CircuitBreaker:      &config.CircuitBreakerConfig{Threshold: 3, ResetAfter: 30 * time.Minute},
	}

	if _, err := q.Enqueue(ctx, queue.EnqueueRequest{Plugin: "echo", Command: pollCommand, SubmittedBy: "existing"}); err != nil {
		t.Fatalf("enqueue existing job: %v", err)
	}

	allowed, reason, err := s.canSchedule(ctx, "echo", pollCommand, pluginConf, config.ScheduleConfig{IfRunning: "skip"})
	if err != nil {
		t.Fatalf("skip canSchedule: %v", err)
	}
	if allowed || reason != "already_running" {
		t.Fatalf("expected already_running for skip, got allowed=%v reason=%q", allowed, reason)
	}

	allowed, reason, err = s.canSchedule(ctx, "echo", pollCommand, pluginConf, config.ScheduleConfig{IfRunning: "cancel"})
	if err != nil {
		t.Fatalf("cancel canSchedule: %v", err)
	}
	if !allowed || reason != "" {
		t.Fatalf("expected allow for cancel, got allowed=%v reason=%q", allowed, reason)
	}
	jobs, total, err := q.ListJobs(ctx, queue.ListJobsFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 1 || len(jobs) != 1 || jobs[0].Status != queue.StatusDead {
		t.Fatalf("expected cancelled job to be dead, got total=%d jobs=%+v", total, jobs)
	}

	if _, err := q.Enqueue(ctx, queue.EnqueueRequest{Plugin: "echo", Command: pollCommand, SubmittedBy: "existing-2"}); err != nil {
		t.Fatalf("enqueue second existing job: %v", err)
	}
	allowed, reason, err = s.canSchedule(ctx, "echo", pollCommand, pluginConf, config.ScheduleConfig{IfRunning: "queue"})
	if err != nil {
		t.Fatalf("queue canSchedule: %v", err)
	}
	if allowed || reason != "outstanding_limit" {
		t.Fatalf("expected outstanding_limit for queue, got allowed=%v reason=%q", allowed, reason)
	}
}

func durationPtr(d time.Duration) *time.Duration { return &d }
