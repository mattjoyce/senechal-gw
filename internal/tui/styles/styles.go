package styles

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Status colours (8-Step Traffic Light).
var (
	StatusHealthy  = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))
	StatusRunning  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8BE9FD"))
	StatusQueued   = lipgloss.NewStyle().Foreground(lipgloss.Color("#BD93F9"))
	StatusWaiting  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1FA8C"))
	StatusDelayed  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C"))
	StatusWarning  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF922B"))
	StatusInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4"))
	StatusFailed   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555"))
)

// UI Chrome (Blue-Orange-Purple Identity).
var (
	TabActive      = lipgloss.NewStyle().Foreground(lipgloss.Color("#61AFEF")).Bold(true).Underline(true)
	TabInactive    = StatusInactive
	SoonAccent     = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5943A")).Bold(true)
	PanelFocused   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#61AFEF"))
	PanelUnfocused = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#3B4D66"))
	HeaderBar    = lipgloss.NewStyle().Background(lipgloss.Color("#282C34")).Padding(0, 1)
	HeaderDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4"))
	HeaderBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#61AFEF")).
			Padding(0, 1)
)

// Pulse & Indicators.
var (
	HeartbeatActive = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF79C6"))
	HeartbeatDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4"))
	ActivityWhite   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	ActivityDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4"))
)

// Time display.
var (
	TimeRecent = StatusHealthy
	TimeMedium = StatusWaiting
	TimeOld    = StatusInactive
	TimeSoon   = StatusRunning
)

// Summary.
var (
	SummaryKey = StatusInactive
	SummaryVal = lipgloss.NewStyle().Bold(true)
)

// Selected row.
var (
	SelectedRow = lipgloss.NewStyle().Background(lipgloss.Color("#3E4452")).Bold(true)
)

// StatusStyle returns the appropriate style for a job/item status string.
func StatusStyle(status string) lipgloss.Style {
	switch status {
	case "ok", "succeeded", "completed", "healthy", "scheduled":
		return StatusHealthy
	case "running":
		return StatusRunning
	case "queued", "pending":
		return StatusQueued
	case "waiting", "idle":
		return StatusWaiting
	case "delayed", "retrying":
		return StatusDelayed
	case "warning", "saturated":
		return StatusWarning
	case "inactive", "dead":
		return StatusInactive
	case "failed", "timed_out", "error":
		return StatusFailed
	default:
		return StatusInactive
	}
}

// RelativeTime formats a past time as a human-readable relative string.
func RelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 0:
		return fmt.Sprintf("in %s", formatDuration(-d))
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// TimeUntil formats a future time as a countdown.
func TimeUntil(t time.Time) string {
	d := time.Until(t)
	if d <= 0 {
		return "now"
	}
	return "in " + formatDuration(d)
}

// TimeStyle returns the appropriate style based on age.
func TimeStyle(t time.Time) lipgloss.Style {
	d := time.Since(t)
	switch {
	case d < 30*time.Second:
		return TimeRecent
	case d < 5*time.Minute:
		return TimeMedium
	default:
		return TimeOld
	}
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
}
