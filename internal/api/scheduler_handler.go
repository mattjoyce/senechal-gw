package api

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/scheduleexpr"
)

func (s *Server) handleSchedulerJobs(w http.ResponseWriter, r *http.Request) {
	cfg := s.config.RuntimeConfig
	if cfg == nil {
		respondJSON(w, http.StatusOK, SchedulerJobsResponse{Jobs: []SchedulerJob{}})
		return
	}

	now := time.Now().UTC()
	jobs := make([]SchedulerJob, 0)

	pluginNames := make([]string, 0, len(cfg.Plugins))
	for name := range cfg.Plugins {
		pluginNames = append(pluginNames, name)
	}
	sort.Strings(pluginNames)

	for _, pluginName := range pluginNames {
		pluginConf := cfg.Plugins[pluginName]
		if !pluginConf.Enabled {
			continue
		}
		schedules := pluginConf.NormalizedSchedules()
		for _, schedule := range schedules {
			command := strings.TrimSpace(schedule.Command)
			if command == "" {
				command = "poll"
			}
			mode := scheduleMode(schedule)
			row := SchedulerJob{
				Plugin:     pluginName,
				ScheduleID: schedule.ID,
				Command:    command,
				Mode:       mode,
				Status:     "scheduled",
			}
			if tz := strings.TrimSpace(schedule.Timezone); tz != "" {
				row.Timezone = tz
			}

			nextRunAt, err := projectedNextRun(schedule, now, s.startedAt)
			if err != nil {
				row.Status = "invalid"
				row.Reason = err.Error()
			} else if nextRunAt != nil {
				row.NextRunAt = nextRunAt
			}
			jobs = append(jobs, row)
		}
	}

	respondJSON(w, http.StatusOK, SchedulerJobsResponse{Jobs: jobs})
}

func scheduleMode(schedule config.ScheduleConfig) string {
	switch {
	case strings.TrimSpace(schedule.Every) != "":
		return "every"
	case strings.TrimSpace(schedule.Cron) != "":
		return "cron"
	case strings.TrimSpace(schedule.At) != "":
		return "at"
	case schedule.After > 0:
		return "after"
	default:
		return "unknown"
	}
}

func projectedNextRun(schedule config.ScheduleConfig, now, startedAt time.Time) (*time.Time, error) {
	switch scheduleMode(schedule) {
	case "every":
		d, err := config.ParseInterval(schedule.Every)
		if err != nil {
			return nil, err
		}
		t := now.Add(d)
		return &t, nil
	case "cron":
		expr, err := scheduleexpr.ParseCron(schedule.Cron)
		if err != nil {
			return nil, err
		}
		loc := time.Local
		if tz := strings.TrimSpace(schedule.Timezone); tz != "" {
			loaded, loadErr := time.LoadLocation(tz)
			if loadErr != nil {
				return nil, loadErr
			}
			loc = loaded
		}
		next, err := expr.NextAfter(now.In(loc))
		if err != nil {
			return nil, err
		}
		nextUTC := next.UTC()
		return &nextUTC, nil
	case "at":
		t, err := time.Parse(time.RFC3339, schedule.At)
		if err != nil {
			return nil, err
		}
		t = t.UTC()
		if t.Before(now) {
			t = now
		}
		return &t, nil
	case "after":
		t := startedAt.UTC().Add(schedule.After)
		if t.Before(now) {
			t = now
		}
		return &t, nil
	default:
		return nil, nil
	}
}
