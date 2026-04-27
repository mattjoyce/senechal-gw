package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/queue"
)

// TestLogBuffer is a bytes.Buffer that can be used to capture log output.
type TestLogBuffer struct {
	bytes.Buffer
}

// NewTestSlogger creates a new *slog.Logger that writes to a TestLogBuffer.
func NewTestSlogger() (*slog.Logger, *TestLogBuffer) {
	var buf TestLogBuffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(handler), &buf
}

func TestCalculateJitteredInterval(t *testing.T) {
	tests := []struct {
		name         string
		baseInterval time.Duration
		jitter       time.Duration
	}{
		{name: "No Jitter", baseInterval: 1 * time.Minute, jitter: 0},
		{name: "Positive Jitter", baseInterval: 5 * time.Minute, jitter: 30 * time.Second},
		{name: "Large Jitter", baseInterval: 1 * time.Hour, jitter: 15 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for range 100 {
				jittered := calculateJitteredInterval(tt.baseInterval, tt.jitter)
				if tt.jitter == 0 {
					assert.Equal(t, tt.baseInterval, jittered)
				} else {
					assert.GreaterOrEqual(t, jittered, tt.baseInterval)
					assert.LessOrEqual(t, jittered, tt.baseInterval+tt.jitter)
				}
			}
		})
	}
}

func TestParseScheduleInterval(t *testing.T) {
	tests := []struct {
		name     string
		every    string
		cron     string
		expected time.Duration
		hasEvery bool
		hasError bool
	}{
		{"5m", "5m", "", 5 * time.Minute, true, false},
		{"hourly", "hourly", "", 1 * time.Hour, true, false},
		{"2w", "2w", "", 14 * 24 * time.Hour, true, false},
		{"cron", "", "*/15 * * * *", 0, false, false},
		{"both", "5m", "*/15 * * * *", 0, false, true},
		{"none", "", "", 0, false, true},
		{"unknown", "foo", "", 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration, hasEvery, err := parseScheduleInterval(tt.every, tt.cron)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, duration)
				assert.Equal(t, tt.hasEvery, hasEvery)
			}
		})
	}
}

func TestCountMissedRuns(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	interval := 5 * time.Minute

	assert.Equal(t, 0, countMissedRuns(base, base, interval))
	assert.Equal(t, 0, countMissedRuns(base, base.Add(4*time.Minute), interval))
	assert.Equal(t, 1, countMissedRuns(base, base.Add(5*time.Minute), interval))
	assert.Equal(t, 3, countMissedRuns(base, base.Add(15*time.Minute), interval))
}

func TestScheduleConstraintsAllowAt(t *testing.T) {
	now := time.Date(2026, 3, 2, 10, 30, 0, 0, time.UTC)

	ok, reason, err := scheduleConstraintsAllowAt(config.ScheduleConfig{}, now)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "", reason)

	ok, reason, err = scheduleConstraintsAllowAt(config.ScheduleConfig{OnlyBetween: "08:00-12:00"}, now)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "", reason)

	ok, reason, err = scheduleConstraintsAllowAt(config.ScheduleConfig{OnlyBetween: "11:00-12:00"}, now)
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "outside_time_window", reason)

	ok, reason, err = scheduleConstraintsAllowAt(config.ScheduleConfig{NotOn: []any{"monday"}}, now)
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "outside_day_constraint", reason)

	ok, reason, err = scheduleConstraintsAllowAt(config.ScheduleConfig{Timezone: "UTC", NotOn: []any{1}}, now)
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "outside_day_constraint", reason)

	_, _, err = scheduleConstraintsAllowAt(config.ScheduleConfig{OnlyBetween: "bad-window"}, now)
	assert.Error(t, err)

	_, _, err = scheduleConstraintsAllowAt(config.ScheduleConfig{Timezone: "Mars/Phobos"}, now)
	assert.Error(t, err)
}

// stubQueueService satisfies QueueService with minimal no-op implementations.
type stubQueueService struct{}

func (s *stubQueueService) Enqueue(_ context.Context, _ queue.EnqueueRequest) (string, error) {
	return "", nil
}
func (s *stubQueueService) CountOutstandingJobs(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}
func (s *stubQueueService) CountOutstandingJobsBySubmitter(_ context.Context, _, _, _ string) (int, error) {
	return 0, nil
}
func (s *stubQueueService) CancelOutstandingJobs(_ context.Context, _, _, _ string) (int, error) {
	return 0, nil
}
func (s *stubQueueService) LatestCompletedCommandResult(_ context.Context, _, _, _ string) (*queue.CommandResult, error) {
	return nil, nil
}
func (s *stubQueueService) GetCircuitBreaker(_ context.Context, _, _ string) (*queue.CircuitBreaker, error) {
	return nil, nil
}
func (s *stubQueueService) UpsertCircuitBreaker(_ context.Context, _ queue.CircuitBreaker) error {
	return nil
}
func (s *stubQueueService) RecordCircuitBreakerTransition(_ context.Context, _ queue.CircuitBreakerTransition) error {
	return nil
}
func (s *stubQueueService) ResetCircuitBreaker(_ context.Context, _, _ string) error { return nil }
func (s *stubQueueService) GetScheduleEntryState(_ context.Context, _, _ string) (*queue.ScheduleEntryState, error) {
	return nil, nil
}
func (s *stubQueueService) UpsertScheduleEntryState(_ context.Context, _ queue.ScheduleEntryState) error {
	return nil
}
func (s *stubQueueService) FindJobsByStatus(_ context.Context, _ queue.Status) ([]*queue.Job, error) {
	return nil, nil
}
func (s *stubQueueService) UpdateJobForRecovery(_ context.Context, _ string, _ queue.Status, _ int, _ *time.Time, _ string) error {
	return nil
}
func (s *stubQueueService) CompleteWithResult(_ context.Context, _ string, _ queue.Status, _ json.RawMessage, _, _ *string) error {
	return nil
}
func (s *stubQueueService) PruneJobLogs(_ context.Context, _ time.Duration) error { return nil }

type circuitQueueStub struct {
	stubQueueService
	breaker     *queue.CircuitBreaker
	latest      *queue.CommandResult
	transitions []queue.CircuitBreakerTransition
}

func (s *circuitQueueStub) LatestCompletedCommandResult(_ context.Context, _, _, _ string) (*queue.CommandResult, error) {
	return s.latest, nil
}

func (s *circuitQueueStub) GetCircuitBreaker(_ context.Context, _, _ string) (*queue.CircuitBreaker, error) {
	return s.breaker, nil
}

func (s *circuitQueueStub) UpsertCircuitBreaker(_ context.Context, cb queue.CircuitBreaker) error {
	copied := cb
	s.breaker = &copied
	return nil
}

func (s *circuitQueueStub) RecordCircuitBreakerTransition(_ context.Context, transition queue.CircuitBreakerTransition) error {
	s.transitions = append(s.transitions, transition)
	return nil
}

func TestReconcileCircuitBreakerRecordsOpenTransition(t *testing.T) {
	logger, _ := NewTestSlogger()
	cfg := config.Defaults()
	stub := &circuitQueueStub{
		latest: &queue.CommandResult{
			JobID:       "job-failed",
			Status:      queue.StatusFailed,
			CompletedAt: time.Now().UTC(),
		},
	}
	sched := New(cfg, stub, nil, logger)

	latest, err := sched.reconcileCircuitBreaker(context.Background(), "echo", "poll", config.PluginConf{
		CircuitBreaker: &config.CircuitBreakerConfig{Threshold: 1, ResetAfter: time.Minute},
	})
	assert.NoError(t, err)
	assert.Equal(t, "job-failed", latest.JobID)
	assert.NotNil(t, stub.breaker)
	assert.Equal(t, queue.CircuitOpen, stub.breaker.State)
	assert.Equal(t, 1, len(stub.transitions))
	assert.Equal(t, queue.CircuitTransitionFailureThreshold, stub.transitions[0].Reason)
	assert.Equal(t, queue.CircuitClosed, *stub.transitions[0].FromState)
	assert.Equal(t, queue.CircuitOpen, stub.transitions[0].ToState)
	assert.Equal(t, "job-failed", *stub.transitions[0].JobID)
}

func TestCanScheduleRecordsHalfOpenTransition(t *testing.T) {
	logger, _ := NewTestSlogger()
	cfg := config.Defaults()
	openedAt := time.Now().UTC().Add(-2 * time.Minute)
	lastJobID := "job-open"
	stub := &circuitQueueStub{
		breaker: &queue.CircuitBreaker{
			Plugin:       "echo",
			Command:      "poll",
			State:        queue.CircuitOpen,
			FailureCount: 3,
			OpenedAt:     &openedAt,
			LastJobID:    &lastJobID,
		},
	}
	sched := New(cfg, stub, nil, logger)

	ok, reason, err := sched.canSchedule(context.Background(), "echo", "poll", config.PluginConf{
		CircuitBreaker: &config.CircuitBreakerConfig{Threshold: 3, ResetAfter: time.Minute},
	}, config.ScheduleConfig{})
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "", reason)
	assert.Equal(t, queue.CircuitHalfOpen, stub.breaker.State)
	assert.Equal(t, 1, len(stub.transitions))
	assert.Equal(t, queue.CircuitTransitionCooldownElapsed, stub.transitions[0].Reason)
	assert.Equal(t, queue.CircuitOpen, *stub.transitions[0].FromState)
	assert.Equal(t, queue.CircuitHalfOpen, stub.transitions[0].ToState)
	assert.Equal(t, lastJobID, *stub.transitions[0].JobID)
}

// pollGuardQueueStub records calls to the scheduler's outstanding-poll query
// path so tests can assert it is the sole source of truth after Sprint 15.
type pollGuardQueueStub struct {
	stubQueueService
	bySubmitterCalls       int
	bySubmitterOutstanding int
	lastSubmittedBy        string
	unfilteredCalls        int
}

func (s *pollGuardQueueStub) CountOutstandingJobsBySubmitter(_ context.Context, _, _, submittedBy string) (int, error) {
	s.bySubmitterCalls++
	s.lastSubmittedBy = submittedBy
	return s.bySubmitterOutstanding, nil
}

func (s *pollGuardQueueStub) CountOutstandingJobs(_ context.Context, _, _ string) (int, error) {
	s.unfilteredCalls++
	return 0, nil
}

// TestCanSchedulePollSkipUsesQueueAsSoleAuthority verifies the scheduler
// refuses to enqueue a duplicate poll when the queue reports an outstanding
// scheduler-submitted job. Sprint 15 removed the in-memory poll view, so the
// queue is the only thing that can answer "is this plugin already polling?".
func TestCanSchedulePollSkipUsesQueueAsSoleAuthority(t *testing.T) {
	logger, _ := NewTestSlogger()
	cfg := config.Defaults()
	stub := &pollGuardQueueStub{bySubmitterOutstanding: 1}
	sched := New(cfg, stub, nil, logger)

	ok, reason, err := sched.canSchedule(context.Background(), "echo", "poll", config.PluginConf{}, config.ScheduleConfig{})
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "already_running", reason)
	assert.Equal(t, 1, stub.bySubmitterCalls)
	assert.Equal(t, 0, stub.unfilteredCalls)
	assert.Equal(t, cfg.Service.Name, stub.lastSubmittedBy)
}

// TestCanSchedulePollQueueRespectsParallelismBudget verifies the queue policy
// honors the per-plugin parallelism limit when the queue alone reports the
// current outstanding count.
func TestCanSchedulePollQueueRespectsParallelismBudget(t *testing.T) {
	logger, _ := NewTestSlogger()
	cfg := config.Defaults()
	stub := &pollGuardQueueStub{bySubmitterOutstanding: 2}
	sched := New(cfg, stub, nil, logger)

	ok, reason, err := sched.canSchedule(context.Background(), "echo", "poll", config.PluginConf{
		MaxOutstandingPolls: 2,
	}, config.ScheduleConfig{IfRunning: "queue"})
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "outstanding_limit", reason)
	assert.Equal(t, 1, stub.bySubmitterCalls)
	assert.Equal(t, 0, stub.unfilteredCalls)
}

// TestCanScheduleNonPollUsesUnfilteredCount verifies non-poll commands query
// the queue without a submitter filter, since their dedupe semantics apply
// across all submitters.
func TestCanScheduleNonPollUsesUnfilteredCount(t *testing.T) {
	logger, _ := NewTestSlogger()
	cfg := config.Defaults()
	stub := &pollGuardQueueStub{}
	sched := New(cfg, stub, nil, logger)

	ok, _, err := sched.canSchedule(context.Background(), "echo", "handle", config.PluginConf{}, config.ScheduleConfig{})
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 0, stub.bySubmitterCalls)
	assert.Equal(t, 1, stub.unfilteredCalls)
}

func TestReconcileCircuitBreakerRecordsCloseTransition(t *testing.T) {
	logger, _ := NewTestSlogger()
	cfg := config.Defaults()
	lastJobID := "job-failed"
	stub := &circuitQueueStub{
		breaker: &queue.CircuitBreaker{
			Plugin:       "echo",
			Command:      "poll",
			State:        queue.CircuitOpen,
			FailureCount: 3,
			LastJobID:    &lastJobID,
		},
		latest: &queue.CommandResult{
			JobID:       "job-success",
			Status:      queue.StatusSucceeded,
			CompletedAt: time.Now().UTC(),
		},
	}
	sched := New(cfg, stub, nil, logger)

	latest, err := sched.reconcileCircuitBreaker(context.Background(), "echo", "poll", config.PluginConf{
		CircuitBreaker: &config.CircuitBreakerConfig{Threshold: 3, ResetAfter: time.Minute},
	})
	assert.NoError(t, err)
	assert.Equal(t, "job-success", latest.JobID)
	assert.Equal(t, queue.CircuitClosed, stub.breaker.State)
	assert.Equal(t, 1, len(stub.transitions))
	assert.Equal(t, queue.CircuitTransitionSuccess, stub.transitions[0].Reason)
	assert.Equal(t, queue.CircuitOpen, *stub.transitions[0].FromState)
	assert.Equal(t, queue.CircuitClosed, stub.transitions[0].ToState)
	assert.Equal(t, "job-success", *stub.transitions[0].JobID)
}

// recoveryQueueStub records calls for recovery-path assertions.
type recoveryQueueStub struct {
	stubQueueService
	mu              sync.Mutex
	jobs            []*queue.Job
	completedJobIDs []string
	completedStatus []queue.Status
	recoveredJobIDs []string
	recoveredStatus []queue.Status
}

func (r *recoveryQueueStub) FindJobsByStatus(_ context.Context, status queue.Status) ([]*queue.Job, error) {
	if status == queue.StatusRunning {
		return r.jobs, nil
	}
	return nil, nil
}

func (r *recoveryQueueStub) UpdateJobForRecovery(_ context.Context, jobID string, newStatus queue.Status, _ int, _ *time.Time, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recoveredJobIDs = append(r.recoveredJobIDs, jobID)
	r.recoveredStatus = append(r.recoveredStatus, newStatus)
	return nil
}

func (r *recoveryQueueStub) CompleteWithResult(_ context.Context, jobID string, status queue.Status, _ json.RawMessage, _, _ *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.completedJobIDs = append(r.completedJobIDs, jobID)
	r.completedStatus = append(r.completedStatus, status)
	return nil
}

func TestRecoverOrphanedJobs_DeadJobFiresHook(t *testing.T) {
	logger, _ := NewTestSlogger()
	cfg := config.Defaults()

	stub := &recoveryQueueStub{
		jobs: []*queue.Job{{
			ID:          "dead-job-1",
			Plugin:      "email_handler",
			Command:     "handle",
			Attempt:     4,
			MaxAttempts: 4, // attempt will be incremented to 5 > max → dead
		}},
	}

	var hookCalls []map[string]any
	var hookMu sync.Mutex

	sched := New(cfg, stub, nil, logger)
	sched.SetRecoveryHook(func(_ context.Context, job *queue.Job, signal string, payload map[string]any) {
		hookMu.Lock()
		defer hookMu.Unlock()
		hookCalls = append(hookCalls, map[string]any{
			"job_id":  job.ID,
			"signal":  signal,
			"payload": payload,
		})
	})

	ctx := context.Background()
	err := sched.recoverOrphanedJobs(ctx)
	assert.NoError(t, err)

	// Should have called CompleteWithResult for the dead job.
	assert.Equal(t, 1, len(stub.completedJobIDs), "expected 1 CompleteWithResult call")
	assert.Equal(t, "dead-job-1", stub.completedJobIDs[0])
	assert.Equal(t, queue.StatusDead, stub.completedStatus[0])

	// Recovery hook should have fired exactly once.
	hookMu.Lock()
	defer hookMu.Unlock()
	assert.Equal(t, 1, len(hookCalls), "expected 1 recovery hook call")
	assert.Equal(t, "dead-job-1", hookCalls[0]["job_id"])
	assert.Equal(t, "job.failed", hookCalls[0]["signal"])

	payload := hookCalls[0]["payload"].(map[string]any)
	assert.Equal(t, true, payload["recovery"], "hook payload must include recovery=true")
	assert.Equal(t, "email_handler", payload["plugin"])
}

func TestRecoverOrphanedJobs_RequeuedJobDoesNotFireHook(t *testing.T) {
	logger, _ := NewTestSlogger()
	cfg := config.Defaults()

	stub := &recoveryQueueStub{
		jobs: []*queue.Job{{
			ID:          "retry-job-1",
			Plugin:      "email_handler",
			Command:     "handle",
			Attempt:     1,
			MaxAttempts: 4, // attempt will be incremented to 2 ≤ max → re-queue
		}},
	}

	hookCalled := false
	sched := New(cfg, stub, nil, logger)
	sched.SetRecoveryHook(func(_ context.Context, _ *queue.Job, _ string, _ map[string]any) {
		hookCalled = true
	})

	ctx := context.Background()
	err := sched.recoverOrphanedJobs(ctx)
	assert.NoError(t, err)

	// Should have re-queued, not completed.
	assert.Equal(t, 0, len(stub.completedJobIDs), "dead-path CompleteWithResult should not be called")
	assert.Equal(t, 1, len(stub.recoveredJobIDs))
	assert.Equal(t, queue.StatusQueued, stub.recoveredStatus[0])

	// Hook must NOT fire for re-queued jobs.
	assert.False(t, hookCalled, "recovery hook must not fire for re-queued jobs")
}
