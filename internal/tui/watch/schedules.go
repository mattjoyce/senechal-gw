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

func renderSchedules(schedules map[string]*ScheduleState, theme Theme, width int) string {
	innerWidth := width - 4

	if len(schedules) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left,
			theme.Title.Render("SCHEDULES"),
			theme.Dim.Render("  No schedule state observed yet..."),
		)
		return theme.Border.Width(innerWidth).Render(content)
	}

	keys := sortedScheduleKeys(schedules)
	var lines []string
	for i, key := range keys {
		if i >= 8 {
			break
		}
		lines = append(lines, renderScheduleRow(schedules[key], theme))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		append([]string{theme.Title.Render("SCHEDULES")}, lines...)...,
	)
	return theme.Border.Width(innerWidth).Render(content)
}

func renderScheduleRow(s *ScheduleState, theme Theme) string {
	status := theme.Dim.Render("[unknown]")
	switch s.Status {
	case "scheduled":
		status = theme.StatusRunning.Render("[scheduled]")
	case "waiting":
		status = theme.Highlight.Render("[waiting]")
	case "skipped":
		status = theme.StatusFailed.Render("[skipped]")
	}

	nextRun := "next: -"
	if !s.NextRunAt.IsZero() {
		nextRun = fmt.Sprintf("next: %s (%s)",
			s.NextRunAt.Local().Format("15:04:05"),
			formatCountdown(time.Until(s.NextRunAt)),
		)
	}

	reason := ""
	if s.Reason != "" && s.Reason != "not_due" {
		reason = " " + theme.Dim.Render("reason="+s.Reason)
	}

	name := fmt.Sprintf("%s/%s [%s]", s.Plugin, s.Command, s.ScheduleID)
	return fmt.Sprintf(" %-28s %s %s%s", name, status, theme.Dim.Render(nextRun), reason)
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
		return "due now"
	}
	until = until.Round(time.Second)
	if until < time.Minute {
		return fmt.Sprintf("in %ds", int(until.Seconds()))
	}
	if until < time.Hour {
		return fmt.Sprintf("in %dm%02ds", int(until.Minutes()), int(until.Seconds())%60)
	}
	return fmt.Sprintf("in %dh%02dm", int(until.Hours()), int(until.Minutes())%60)
}
