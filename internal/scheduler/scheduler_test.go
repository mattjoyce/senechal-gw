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
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	return slog.New(handler), &buf
}

func TestCalculateJitteredInterval(t *testing.T) {
	tests := []struct {
		name         string
		baseInterval time.Duration
		jitter       time.Duration
	}{
		{
			name:         "No Jitter",
			baseInterval: 1 * time.Minute,
			jitter:       0,
		},
		{
			name:         "Positive Jitter",
			baseInterval: 5 * time.Minute,
			jitter:       30 * time.Second,
		},
		{
			name:         "Large Jitter",
			baseInterval: 1 * time.Hour,
			jitter:       15 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to observe randomness within the jitter range
			for i := 0; i < 100; i++ {
				jittered := calculateJitteredInterval(tt.baseInterval, tt.jitter)

				// For no jitter, it should be exactly the base interval
				if tt.jitter == 0 {
					assert.Equal(t, tt.baseInterval, jittered, "Jittered interval should match base without jitter")
				} else {
					// With jitter, it should be between base and base + jitter
					assert.GreaterOrEqual(t, jittered, tt.baseInterval, "Jittered interval should not be less than base")
					assert.LessOrEqual(t, jittered, tt.baseInterval+tt.jitter, "Jittered interval should not exceed base + jitter")
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
		{"15m", "15m", 15 * time.Minute, false},
		{"30m", "30m", 30 * time.Minute, false},
		{"7m", "7m", 7 * time.Minute, false},
		{"hourly", "hourly", 1 * time.Hour, false},
		{"2h", "2h", 2 * time.Hour, false},
		{"13h", "13h", 13 * time.Hour, false},
		{"6h", "6h", 6 * time.Hour, false},
		{"3d", "3d", 3 * 24 * time.Hour, false},
		{"2w", "2w", 14 * 24 * time.Hour, false},
		{"daily", "daily", 24 * time.Hour, false},
		{"weekly", "weekly", 7 * 24 * time.Hour, false},
		{"monthly", "monthly", 30 * 24 * time.Hour, false}, // Approximation
		{"unknown", "foo", 0, true},
		{"invalid duration", "abc", 0, true},
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
	s := New(cfg, mockQueue, slogger)
	ctx := context.Background()

	t.Run("No orphaned jobs", func(t *testing.T) {
		mockQueue.EXPECT().FindJobsByStatus(ctx, queue.StatusRunning).Return([]*queue.Job{}, nil)
		err := s.recoverOrphanedJobs(ctx)
		assert.NoError(t, err)
	})

	t.Run("Orphaned jobs - some re-queued, some dead", func(t *testing.T) {
		logBuf.Reset() // Clear buffer for this test

		job1 := &queue.Job{
			ID:          "job1",
			Plugin:      "pluginA",
			Command:     "poll",
			Status:      queue.StatusRunning,
			Attempt:     1,
			MaxAttempts: 3,
			SubmittedBy: "test",
		}
		job2 := &queue.Job{
			ID:          "job2",
			Plugin:      "pluginB",
			Command:     "handle",
			Status:      queue.StatusRunning,
			Attempt:     3,
			MaxAttempts: 3,
			SubmittedBy: "test",
		}

		mockQueue.EXPECT().FindJobsByStatus(ctx, queue.StatusRunning).Return([]*queue.Job{job1, job2}, nil)

		// Expect job1 to be re-queued
		mockQueue.EXPECT().UpdateJobForRecovery(
			ctx,
			"job1",
			queue.StatusQueued,
			2, // Attempt incremented
			nil,
			"",
		).Return(nil)

		// Expect job2 to be marked dead
		mockQueue.EXPECT().UpdateJobForRecovery(
			ctx,
			"job2",
			queue.StatusDead,
			4, // Attempt incremented
			nil,
			gomock.Any(), // lastError will be a formatted string
		).Return(nil)

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

	t.Run("UpdateJobForRecovery returns error", func(t *testing.T) {
		logBuf.Reset()
		job1 := &queue.Job{
			ID:          "job1",
			Plugin:      "pluginA",
			Command:     "poll",
			Status:      queue.StatusRunning,
			Attempt:     1,
			MaxAttempts: 3,
			SubmittedBy: "test",
		}

		mockQueue.EXPECT().FindJobsByStatus(ctx, queue.StatusRunning).Return([]*queue.Job{job1}, nil)
		mockQueue.EXPECT().UpdateJobForRecovery(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("update error"))

		err := s.recoverOrphanedJobs(ctx)
		assert.NoError(t, err) // Error from update is logged, but recovery process continues for other jobs
		assert.Contains(t, logBuf.String(), "Failed to update orphaned job during recovery")
	})
}

func TestSchedulerTick(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueue := mocks.NewMockQueueService(ctrl)
	slogger, logBuf := NewTestSlogger()

	cfg := config.Defaults()
	cfg.Service.TickInterval = 1 * time.Second
	cfg.Service.JobLogRetention = 24 * time.Hour

	// Configure some plugins
	cfg.Plugins = map[string]config.PluginConf{
		"enabled_plugin_short_interval": {
			Enabled: true,
			Schedule: &config.ScheduleConfig{
				Every:  "1s",
				Jitter: 100 * time.Millisecond,
			},
			Retry: &config.RetryConfig{MaxAttempts: 3},
		},
		"enabled_plugin_long_interval": {
			Enabled: true,
			Schedule: &config.ScheduleConfig{
				Every:  "hourly",
				Jitter: 5 * time.Minute,
			},
			Retry: &config.RetryConfig{MaxAttempts: 3},
		},
		"disabled_plugin": {
			Enabled: false,
			Schedule: &config.ScheduleConfig{
				Every: "5m",
			},
		},
		"plugin_no_schedule": {
			Enabled: true,
		},
	}

	s := New(cfg, mockQueue, slogger)
	ctx := context.Background()

	t.Run("Normal tick operations", func(t *testing.T) {
		logBuf.Reset()

		// Expect enqueue for enabled_plugin_long_interval (alphabetically first)
		mockQueue.EXPECT().Enqueue(
			gomock.Any(),
			gomock.Any(),
		).DoAndReturn(func(_ context.Context, req queue.EnqueueRequest) (string, error) {
			assert.Equal(t, "enabled_plugin_long_interval", req.Plugin)
			assert.Equal(t, "poll", req.Command)
			assert.NotNil(t, req.DedupeKey)
			assert.Equal(t, "poll:enabled_plugin_long_interval", *req.DedupeKey)
			assert.Equal(t, 3, req.MaxAttempts)
			return "job_id_long", nil
		}).Times(1)

		// Expect enqueue for enabled_plugin_short_interval
		mockQueue.EXPECT().Enqueue(
			gomock.Any(),
			gomock.Any(),
		).DoAndReturn(func(_ context.Context, req queue.EnqueueRequest) (string, error) {
			assert.Equal(t, "enabled_plugin_short_interval", req.Plugin)
			assert.Equal(t, "poll", req.Command)
			assert.NotNil(t, req.DedupeKey)
			assert.Equal(t, "poll:enabled_plugin_short_interval", *req.DedupeKey)
			assert.Equal(t, 3, req.MaxAttempts)
			return "job_id_short", nil
		}).Times(1)

		// Expect PruneJobLogs to be called
		mockQueue.EXPECT().PruneJobLogs(ctx, cfg.Service.JobLogRetention).Return(nil).Times(1)

		// Perform a single tick
		s.tick(ctx)

		assert.Contains(t, logBuf.String(), "Enqueued poll job", "short interval should be enqueued")
		assert.Contains(t, logBuf.String(), `plugin":"enabled_plugin_short_interval","job_id":"job_id_short"`, "short interval enqueue log should be present")
		assert.Contains(t, logBuf.String(), "Enqueued poll job", "long interval should be enqueued")
		assert.Contains(t, logBuf.String(), `plugin":"enabled_plugin_long_interval","job_id":"job_id_long"`, "long interval enqueue log should be present")

		// Verify no enqueue for disabled or no-schedule plugins (checked by mock expectations)
		assert.NotContains(t, logBuf.String(), "disabled_plugin")
		assert.NotContains(t, logBuf.String(), "plugin_no_schedule")
	})

	t.Run("PruneJobLogs returns error", func(t *testing.T) {
		logBuf.Reset()
		// Expect enqueue for both enabled plugins
		mockQueue.EXPECT().Enqueue(gomock.Any(), gomock.Any()).Return("job_id_short_2", nil).Times(1)
		mockQueue.EXPECT().Enqueue(gomock.Any(), gomock.Any()).Return("job_id_long_2", nil).Times(1)

		mockQueue.EXPECT().PruneJobLogs(ctx, cfg.Service.JobLogRetention).Return(errors.New("prune error")).Times(1)
		s.tick(ctx)
		assert.Contains(t, logBuf.String(), "Failed to prune job logs")
	})

	t.Run("Enqueue error", func(t *testing.T) {
		logBuf.Reset()
		// Expect enqueue for both plugins, but short interval fails
		mockQueue.EXPECT().Enqueue(
			gomock.Any(),
			gomock.Any(),
		).DoAndReturn(func(_ context.Context, req queue.EnqueueRequest) (string, error) {
			if req.Plugin == "enabled_plugin_short_interval" {
				return "", errors.New("enqueue error") // Make short interval enqueue fail
			}
			return "job_id_long_3", nil // Long interval enqueue succeeds
		}).Times(2) // Two enqueues are expected

		mockQueue.EXPECT().PruneJobLogs(ctx, cfg.Service.JobLogRetention).Return(nil).Times(1)
		s.tick(ctx)
		assert.Contains(t, logBuf.String(), "Failed to enqueue poll job")
		assert.Contains(t, logBuf.String(), `plugin":"enabled_plugin_short_interval"`, "Error log should specify short interval plugin")
		assert.Contains(t, logBuf.String(), `plugin":"enabled_plugin_long_interval"`, "Long interval enqueue should succeed and be logged")
	})
}
