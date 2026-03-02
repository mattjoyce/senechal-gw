package watch

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/events"
)

// ScheduleState tracks one scheduler entry in the watch TUI.
type ScheduleState struct {
	Plugin     string
	ScheduleID string
	Command    string
	Status     string
	Reason     string
	NextRunAt  time.Time
	LastSeen   time.Time
}

func updateScheduleState(schedules map[string]*ScheduleState, e events.Event) {
	if schedules == nil {
		return
	}
	if e.Type != "scheduler.skipped" && e.Type != "scheduler.scheduled" {
		return
	}

	data := make(map[string]any)
	_ = json.Unmarshal(e.Data, &data)

	plugin, _ := data["plugin"].(string)
	if plugin == "" {
		return
	}
	scheduleID, _ := data["schedule_id"].(string)
	if strings.TrimSpace(scheduleID) == "" {
		scheduleID = "default"
	}
	command, _ := data["command"].(string)
	if strings.TrimSpace(command) == "" {
		command = "poll"
	}

	key := scheduleKey(plugin, scheduleID, command)
	state, ok := schedules[key]
	if !ok {
		state = &ScheduleState{
			Plugin:     plugin,
			ScheduleID: scheduleID,
			Command:    command,
		}
		schedules[key] = state
	}
	state.LastSeen = time.Now()

	switch e.Type {
	case "scheduler.scheduled":
		state.Status = "scheduled"
		state.Reason = ""

	case "scheduler.skipped":
		reason, _ := data["reason"].(string)
		state.Reason = reason
		if reason == "not_due" {
			state.Status = "waiting"
		} else {
			state.Status = "skipped"
		}
		if rawNextRunAt, ok := data["next_run_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, rawNextRunAt); err == nil {
				state.NextRunAt = t
			}
		}
	}
}

func applyScheduleSnapshot(schedules map[string]*ScheduleState, snap schedulerSnapshotMsg) {
	if schedules == nil {
		return
	}
	now := time.Now()
	for _, job := range snap.Jobs {
		plugin := strings.TrimSpace(job.Plugin)
		if plugin == "" {
			continue
		}
		scheduleID := strings.TrimSpace(job.ScheduleID)
		if scheduleID == "" {
			scheduleID = "default"
		}
		command := strings.TrimSpace(job.Command)
		if command == "" {
			command = "poll"
		}

		key := scheduleKey(plugin, scheduleID, command)
		state, ok := schedules[key]
		if !ok {
			state = &ScheduleState{
				Plugin:     plugin,
				ScheduleID: scheduleID,
				Command:    command,
			}
			schedules[key] = state
		}
		state.LastSeen = now
		state.Reason = strings.TrimSpace(job.Reason)

		if job.NextRunAt != nil {
			state.NextRunAt = *job.NextRunAt
		}

		switch strings.TrimSpace(job.Status) {
		case "invalid":
			state.Status = "skipped"
			if state.Reason == "" {
				state.Reason = "invalid_schedule"
			}
		case "exhausted":
			state.Status = "skipped"
			if state.Reason == "" {
				state.Reason = "schedule_exhausted"
			}
		default:
			if !state.NextRunAt.IsZero() && now.Before(state.NextRunAt) {
				state.Status = "waiting"
				if state.Reason == "" {
					state.Reason = "not_due"
				}
			} else {
				state.Status = "scheduled"
				if state.Reason == "" {
					state.Reason = ""
				}
			}
		}
	}
}

func renderSchedules(schedules map[string]*ScheduleState, theme Theme, width int, height int, scroll int) string {
	innerWidth := width - 2

	if len(schedules) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left,
			theme.Dim.Render("  No schedule state observed yet..."),
		)
		return theme.Border.Width(innerWidth).Height(height).Render(content)
	}

	visibleHeight := height - 2
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	keys := sortedScheduleKeys(schedules)
	if scroll >= len(keys) {
		scroll = len(keys) - 1
	}
	if scroll < 0 {
		scroll = 0
	}

	var lines []string
	for i := scroll; i < len(keys) && len(lines) < visibleHeight; i++ {
		lines = append(lines, renderScheduleRow(schedules[keys[i]], theme, width-4))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	if len(keys) > visibleHeight {
		content = renderWithScrollbar(content, scroll, len(keys), visibleHeight, theme)
	}

	return theme.Border.Width(innerWidth).Height(height).Render(content)
}

func renderScheduleRow(s *ScheduleState, theme Theme, width int) string {
	status := theme.Dim.Render("[unk]")
	switch s.Status {
	case "scheduled":
		status = theme.StatusRunning.Render("[sch]")
	case "waiting":
		status = theme.Highlight.Render("[wai]")
	case "skipped":
		status = theme.StatusFailed.Render("[ski]")
	}

	name := fmt.Sprintf("%s/%s", s.Plugin, s.Command)
	if len(name) > width-25 {
		name = name[:width-28] + "..."
	}

	nextRun := "-"
	if !s.NextRunAt.IsZero() {
		nextRun = formatCountdown(time.Until(s.NextRunAt))
	}

	return fmt.Sprintf(" %-*s %s %s", width-15, name, status, theme.Dim.Render(nextRun))
}

func scheduleKey(plugin, scheduleID, command string) string {
	return plugin + "\x00" + scheduleID + "\x00" + command
}

func sortedScheduleKeys(schedules map[string]*ScheduleState) []string {
	keys := make([]string, 0, len(schedules))
	for key := range schedules {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatCountdown(until time.Duration) string {
	if until <= 0 {
		return "due"
	}
	until = until.Round(time.Second)
	if until < time.Minute {
		return fmt.Sprintf("%ds", int(until.Seconds()))
	}
	if until < time.Hour {
		return fmt.Sprintf("%dm", int(until.Minutes()))
	}
	return fmt.Sprintf("%dh", int(until.Hours()))
}
