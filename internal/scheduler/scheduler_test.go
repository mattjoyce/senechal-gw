package scheduler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/scheduler/mocks"
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

func TestParseScheduleEvery(t *testing.T) {
	tests := []struct {
		name     string
		every    string
		expected time.Duration
		hasError bool
	}{
		{"5m", "5m", 5 * time.Minute, false},
		{"hourly", "hourly", 1 * time.Hour, false},
		{"2w", "2w", 14 * 24 * time.Hour, false},
		{"unknown", "foo", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration, err := parseScheduleEvery(tt.every)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, duration)
			}
		})
	}
}

func TestRecoverOrphanedJobs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueue := mocks.NewMockQueueService(ctrl)
	slogger, logBuf := NewTestSlogger()
	cfg := config.Defaults()
	s := New(cfg, mockQueue, events.NewHub(32), slogger)
	ctx := context.Background()

	t.Run("No orphaned jobs", func(t *testing.T) {
		mockQueue.EXPECT().FindJobsByStatus(ctx, queue.StatusRunning).Return([]*queue.Job{}, nil)
		err := s.recoverOrphanedJobs(ctx)
		assert.NoError(t, err)
	})

	t.Run("Orphaned jobs - some re-queued, some dead", func(t *testing.T) {
		logBuf.Reset()

		job1 := &queue.Job{ID: "job1", Plugin: "pluginA", Command: "poll", Status: queue.StatusRunning, Attempt: 1, MaxAttempts: 3, SubmittedBy: "test"}
		job2 := &queue.Job{ID: "job2", Plugin: "pluginB", Command: "handle", Status: queue.StatusRunning, Attempt: 3, MaxAttempts: 3, SubmittedBy: "test"}

		mockQueue.EXPECT().FindJobsByStatus(ctx, queue.StatusRunning).Return([]*queue.Job{job1, job2}, nil)
		mockQueue.EXPECT().UpdateJobForRecovery(ctx, "job1", queue.StatusQueued, 2, nil, "").Return(nil)
		mockQueue.EXPECT().UpdateJobForRecovery(ctx, "job2", queue.StatusDead, 4, nil, gomock.Any()).Return(nil)

		err := s.recoverOrphanedJobs(ctx)
		assert.NoError(t, err)
		assert.Contains(t, logBuf.String(), "Re-queueing orphaned job")
		assert.Contains(t, logBuf.String(), "Marking orphaned job as dead")
	})

	t.Run("FindJobsByStatus returns error", func(t *testing.T) {
		logBuf.Reset()
		mockQueue.EXPECT().FindJobsByStatus(ctx, queue.StatusRunning).Return(nil, errors.New("db error"))
		err := s.recoverOrphanedJobs(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to find running jobs for recovery: db error")
	})
}

func TestSchedulerTickEnqueuesWhenAllowed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueue := mocks.NewMockQueueService(ctrl)
	slogger, logBuf := NewTestSlogger()

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {
			Enabled: true,
			Schedule: &config.ScheduleConfig{
				Every:  "1m",
				Jitter: 100 * time.Millisecond,
			},
			Retry:               &config.RetryConfig{MaxAttempts: 3},
			CircuitBreaker:      &config.CircuitBreakerConfig{Threshold: 3, ResetAfter: 30 * time.Minute},
			MaxOutstandingPolls: 1,
		},
	}

	s := New(cfg, mockQueue, events.NewHub(64), slogger)
	ctx := context.Background()

	mockQueue.EXPECT().GetCircuitBreaker(gomock.Any(), "echo", pollCommand).Return(nil, nil)
	mockQueue.EXPECT().LatestCompletedCommandResult(gomock.Any(), "echo", pollCommand, "ductile").Return(nil, nil)
	mockQueue.EXPECT().GetCircuitBreaker(gomock.Any(), "echo", pollCommand).Return(nil, nil)
	mockQueue.EXPECT().CountOutstandingJobs(gomock.Any(), "echo", pollCommand).Return(0, nil)
	mockQueue.EXPECT().GetScheduleEntryState(gomock.Any(), "echo", "default").Return(nil, nil)
	mockQueue.EXPECT().Enqueue(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req queue.EnqueueRequest) (string, error) {
		assert.Equal(t, "echo", req.Plugin)
		assert.Equal(t, pollCommand, req.Command)
		assert.NotNil(t, req.DedupeKey)
		assert.Equal(t, "echo:poll:default", *req.DedupeKey)
		assert.Equal(t, 3, req.MaxAttempts)
		assert.Equal(t, []byte(`{}`), []byte(req.Payload))
		return "job-1", nil
	})
	mockQueue.EXPECT().PruneJobLogs(gomock.Any(), cfg.Service.JobLogRetention).Return(nil)

	s.tick(ctx)

	assert.Contains(t, logBuf.String(), "Enqueued scheduled job")
	assert.Contains(t, logBuf.String(), `"job_id":"job-1"`)
}

func TestSchedulerTickPollGuardSkipsEnqueue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueue := mocks.NewMockQueueService(ctrl)
	slogger, logBuf := NewTestSlogger()

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {
			Enabled: true,
			Schedule: &config.ScheduleConfig{
				Every: "1m",
			},
			MaxOutstandingPolls: 1,
			CircuitBreaker:      &config.CircuitBreakerConfig{Threshold: 3, ResetAfter: 30 * time.Minute},
		},
	}

	s := New(cfg, mockQueue, events.NewHub(64), slogger)
	ctx := context.Background()

	mockQueue.EXPECT().GetCircuitBreaker(gomock.Any(), "echo", pollCommand).Return(nil, nil)
	mockQueue.EXPECT().LatestCompletedCommandResult(gomock.Any(), "echo", pollCommand, "ductile").Return(nil, nil)
	mockQueue.EXPECT().GetCircuitBreaker(gomock.Any(), "echo", pollCommand).Return(nil, nil)
	mockQueue.EXPECT().CountOutstandingJobs(gomock.Any(), "echo", pollCommand).Return(1, nil)
	mockQueue.EXPECT().GetScheduleEntryState(gomock.Any(), "echo", "default").Return(nil, nil)
	mockQueue.EXPECT().PruneJobLogs(gomock.Any(), cfg.Service.JobLogRetention).Return(nil)

	s.tick(ctx)

	assert.Contains(t, logBuf.String(), "Skipped scheduled job")
}

func TestSchedulerTickRejectsScheduledHandle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueue := mocks.NewMockQueueService(ctrl)
	slogger, logBuf := NewTestSlogger()

	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	cfg.Service.JobLogRetention = 24 * time.Hour
	cfg.Plugins = map[string]config.PluginConf{
		"echo": {
			Enabled: true,
			Schedule: &config.ScheduleConfig{
				Every:   "1m",
				Command: "handle",
			},
		},
	}

	s := New(cfg, mockQueue, events.NewHub(64), slogger)
	ctx := context.Background()

	mockQueue.EXPECT().GetScheduleEntryState(gomock.Any(), "echo", "default").Return(nil, nil)
	mockQueue.EXPECT().UpsertScheduleEntryState(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, state queue.ScheduleEntryState) error {
		assert.Equal(t, "echo", state.Plugin)
		assert.Equal(t, "default", state.ScheduleID)
		assert.Equal(t, "handle", state.Command)
		assert.Equal(t, queue.ScheduleEntryPausedInvalid, state.Status)
		assert.NotNil(t, state.Reason)
		assert.Equal(t, "scheduled_handle_disallowed", *state.Reason)
		return nil
	})
	mockQueue.EXPECT().PruneJobLogs(gomock.Any(), cfg.Service.JobLogRetention).Return(nil)

	s.tick(ctx)

	assert.Contains(t, logBuf.String(), "Scheduled command is not allowed")
}

func TestCircuitBreakerStateTransitions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueue := mocks.NewMockQueueService(ctrl)
	slogger, _ := NewTestSlogger()
	cfg := config.Defaults()
	cfg.Service.Name = "ductile"
	s := New(cfg, mockQueue, events.NewHub(64), slogger)
	ctx := context.Background()

	pluginConf := config.PluginConf{
		CircuitBreaker:      &config.CircuitBreakerConfig{Threshold: 2, ResetAfter: 30 * time.Minute},
		MaxOutstandingPolls: 1,
	}

	job1 := queue.PollResult{JobID: "job-1", Status: queue.StatusFailed, CompletedAt: time.Now().UTC().Add(-4 * time.Minute)}
	mockQueue.EXPECT().GetCircuitBreaker(gomock.Any(), "echo", pollCommand).Return(nil, nil)
	mockQueue.EXPECT().LatestCompletedCommandResult(gomock.Any(), "echo", pollCommand, "ductile").Return(&job1, nil)
	mockQueue.EXPECT().UpsertCircuitBreaker(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, cb queue.CircuitBreaker) error {
		assert.Equal(t, queue.CircuitClosed, cb.State)
		assert.Equal(t, 1, cb.FailureCount)
		assert.NotNil(t, cb.LastJobID)
		assert.Equal(t, "job-1", *cb.LastJobID)
		return nil
	})
	assert.NoError(t, s.reconcileCircuitBreaker(ctx, "echo", pollCommand, pluginConf))

	job1ID := "job-1"
	job2 := queue.PollResult{JobID: "job-2", Status: queue.StatusFailed, CompletedAt: time.Now().UTC().Add(-3 * time.Minute)}
	mockQueue.EXPECT().GetCircuitBreaker(gomock.Any(), "echo", pollCommand).Return(&queue.CircuitBreaker{Plugin: "echo", Command: pollCommand, State: queue.CircuitClosed, FailureCount: 1, LastJobID: &job1ID}, nil)
	mockQueue.EXPECT().LatestCompletedCommandResult(gomock.Any(), "echo", pollCommand, "ductile").Return(&job2, nil)
	mockQueue.EXPECT().UpsertCircuitBreaker(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, cb queue.CircuitBreaker) error {
		assert.Equal(t, queue.CircuitOpen, cb.State)
		assert.Equal(t, 2, cb.FailureCount)
		assert.NotNil(t, cb.OpenedAt)
		return nil
	})
	assert.NoError(t, s.reconcileCircuitBreaker(ctx, "echo", pollCommand, pluginConf))

	openedAt := time.Now().UTC().Add(-1 * time.Minute)
	mockQueue.EXPECT().GetCircuitBreaker(gomock.Any(), "echo", pollCommand).Return(&queue.CircuitBreaker{Plugin: "echo", Command: pollCommand, State: queue.CircuitOpen, FailureCount: 2, OpenedAt: &openedAt}, nil)
	allowed, reason, err := s.canSchedule(ctx, "echo", pollCommand, pluginConf)
	assert.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, "circuit_open", reason)

	openedAtElapsed := time.Now().UTC().Add(-31 * time.Minute)
	mockQueue.EXPECT().GetCircuitBreaker(gomock.Any(), "echo", pollCommand).Return(&queue.CircuitBreaker{Plugin: "echo", Command: pollCommand, State: queue.CircuitOpen, FailureCount: 2, OpenedAt: &openedAtElapsed}, nil)
	mockQueue.EXPECT().UpsertCircuitBreaker(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, cb queue.CircuitBreaker) error {
		assert.Equal(t, queue.CircuitHalfOpen, cb.State)
		return nil
	})
	mockQueue.EXPECT().CountOutstandingJobs(gomock.Any(), "echo", pollCommand).Return(0, nil)
	allowed, reason, err = s.canSchedule(ctx, "echo", pollCommand, pluginConf)
	assert.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, "", reason)

	job2ID := "job-2"
	job3 := queue.PollResult{JobID: "job-3", Status: queue.StatusSucceeded, CompletedAt: time.Now().UTC().Add(-2 * time.Minute)}
	mockQueue.EXPECT().GetCircuitBreaker(gomock.Any(), "echo", pollCommand).Return(&queue.CircuitBreaker{Plugin: "echo", Command: pollCommand, State: queue.CircuitHalfOpen, FailureCount: 2, LastJobID: &job2ID}, nil)
	mockQueue.EXPECT().LatestCompletedCommandResult(gomock.Any(), "echo", pollCommand, "ductile").Return(&job3, nil)
	mockQueue.EXPECT().UpsertCircuitBreaker(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, cb queue.CircuitBreaker) error {
		assert.Equal(t, queue.CircuitClosed, cb.State)
		assert.Equal(t, 0, cb.FailureCount)
		assert.Nil(t, cb.OpenedAt)
		return nil
	})
	assert.NoError(t, s.reconcileCircuitBreaker(ctx, "echo", pollCommand, pluginConf))
}
