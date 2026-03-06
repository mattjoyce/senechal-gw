package scheduler

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mattjoyce/ductile/internal/config"
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
