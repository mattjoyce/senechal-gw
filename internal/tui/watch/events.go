package watch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/events"
)

func renderEventStream(eventLog []events.Event, theme Theme, width int, height int, scroll int) string {
	innerWidth := width - 2

	if len(eventLog) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left,
			theme.Dim.Render("  Waiting for events..."),
		)
		return theme.Border.Width(innerWidth).Height(height).Render(content)
	}

	visibleHeight := height - 2
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	if scroll >= len(eventLog) {
		scroll = len(eventLog) - 1
	}
	if scroll < 0 {
		scroll = 0
	}

	var lines []string
	for i := scroll; i < len(eventLog) && len(lines) < visibleHeight; i++ {
		lines = append(lines, formatEvent(eventLog[i], theme, width-10))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	if len(eventLog) > visibleHeight {
		content = renderWithScrollbar(content, scroll, len(eventLog), visibleHeight, theme)
	}

	return theme.Border.Width(innerWidth).Height(height).Render(content)
}

func formatEvent(e events.Event, theme Theme, width int) string {
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

	typeWidth := 15
	typeName := e.Type
	if len(typeName) > typeWidth {
		typeName = typeName[:typeWidth-2] + ".."
	}
	typeName = typeStyle.Render(fmt.Sprintf("%-*s", typeWidth, typeName))

	// Extract brief description from data
	desc := extractEventDesc(e)
	maxDescLen := width - typeWidth - 10
	if maxDescLen < 5 {
		maxDescLen = 5
	}
	if len(desc) > maxDescLen {
		desc = desc[:maxDescLen-3] + "..."
	}

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
