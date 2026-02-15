package watch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/events"
)

func renderEventStream(eventLog []events.Event, theme Theme, width int) string {
	innerWidth := width - 4

	if len(eventLog) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left,
			theme.Title.Render("EVENT STREAM"),
			theme.Dim.Render("  Waiting for events..."),
		)
		return theme.Border.Width(innerWidth).Render(content)
	}

	var lines []string
	for i, e := range eventLog {
		if i >= 10 {
			break
		}
		lines = append(lines, formatEvent(e, theme))
	}

	eventsText := lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
	content := lipgloss.JoinVertical(lipgloss.Left,
		theme.Title.Render("EVENT STREAM"),
		eventsText,
	)

	return theme.Border.Width(innerWidth).Render(content)
}

func formatEvent(e events.Event, theme Theme) string {
	ts := theme.Dim.Render(e.At.Format("15:04:05"))

	// Color the event type based on category
	var typeStyle lipgloss.Style
	switch {
	case strings.HasSuffix(e.Type, ".completed"):
		typeStyle = theme.StatusOK
	case strings.HasSuffix(e.Type, ".failed"), strings.HasSuffix(e.Type, ".timed_out"):
		typeStyle = theme.StatusFailed
	case strings.HasSuffix(e.Type, ".started"):
		typeStyle = theme.StatusRunning
	case strings.HasPrefix(e.Type, "scheduler"):
		typeStyle = theme.Highlight
	default:
		typeStyle = theme.Dim
	}

	typeName := typeStyle.Render(fmt.Sprintf("%-20s", e.Type))

	// Extract brief description from data
	desc := extractEventDesc(e)

	return fmt.Sprintf("%s %s %s", ts, typeName, desc)
}

func extractEventDesc(e events.Event) string {
	data := make(map[string]any)
	_ = json.Unmarshal(e.Data, &data)

	var parts []string

	if jobID, ok := data["job_id"].(string); ok {
		if len(jobID) > 8 {
			jobID = jobID[:8]
		}
		parts = append(parts, fmt.Sprintf("[%s]", jobID))
	}

	if plugin, ok := data["plugin"].(string); ok {
		parts = append(parts, plugin)
	}

	if pipeline, ok := data["pipeline"].(string); ok && pipeline != "" {
		parts = append(parts, pipeline)
	}

	if status, ok := data["status"].(string); ok {
		parts = append(parts, status)
	}

	if len(parts) == 0 {
		raw := string(e.Data)
		if len(raw) > 60 {
			raw = raw[:60] + "..."
		}
		return raw
	}

	return strings.Join(parts, " ")
}
