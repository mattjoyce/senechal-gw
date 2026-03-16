package scheduler

import (
	"bytes"
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/workspace"
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

// stubWorkspaceManager is a minimal workspace.Manager that records Cleanup calls.
type stubWorkspaceManager struct {
	cleanupCalls atomic.Int32
	cleanupTTL   atomic.Value // stores time.Duration
}

func (m *stubWorkspaceManager) Create(_ context.Context, _ string) (workspace.Workspace, error) {
	return workspace.Workspace{}, nil
}
func (m *stubWorkspaceManager) Clone(_ context.Context, _, _ string) (workspace.Workspace, error) {
	return workspace.Workspace{}, nil
}
func (m *stubWorkspaceManager) Open(_ context.Context, _ string) (workspace.Workspace, error) {
	return workspace.Workspace{}, nil
}
func (m *stubWorkspaceManager) Cleanup(_ context.Context, olderThan time.Duration) (workspace.CleanupReport, error) {
	m.cleanupCalls.Add(1)
	m.cleanupTTL.Store(olderThan)
	return workspace.CleanupReport{}, nil
}

// stubQueueService satisfies QueueService with minimal no-op implementations.
type stubQueueService struct{}

func (s *stubQueueService) Enqueue(_ context.Context, _ queue.EnqueueRequest) (string, error) {
	return "", nil
}
func (s *stubQueueService) CountOutstandingJobs(_ context.Context, _, _ string) (int, error) {
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
func (s *stubQueueService) PruneJobLogs(_ context.Context, _ time.Duration) error { return nil }

func TestSchedulerJanitorCalledOnTick(t *testing.T) {
	logger, _ := NewTestSlogger()
	cfg := config.Defaults()
	cfg.Plugins = map[string]config.PluginConf{} // no scheduled plugins

	wm := &stubWorkspaceManager{}
	ttl := 48 * time.Hour

	sched := New(cfg, &stubQueueService{}, nil, logger,
		WithWorkspaceJanitor(wm, ttl),
	)

	ctx := context.Background()
	sched.tick(ctx)

	if wm.cleanupCalls.Load() != 1 {
		t.Fatalf("Cleanup() call count = %d, want 1", wm.cleanupCalls.Load())
	}
	got, _ := wm.cleanupTTL.Load().(time.Duration)
	if got != ttl {
		t.Errorf("Cleanup() TTL = %v, want %v", got, ttl)
	}
}

func TestSchedulerJanitorNotCalledWhenNotConfigured(t *testing.T) {
	logger, _ := NewTestSlogger()
	cfg := config.Defaults()
	cfg.Plugins = map[string]config.PluginConf{}

	sched := New(cfg, &stubQueueService{}, nil, logger)
	// No WithWorkspaceJanitor — workspaceManager is nil.

	ctx := context.Background()
	sched.tick(ctx)
	// Just verify no panic; no Cleanup to assert.
}
