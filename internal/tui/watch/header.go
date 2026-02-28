package watch

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// HealthState tracks gateway health from /healthz polling.
type HealthState struct {
	Status        string
	UptimeSeconds int64
	QueueDepth    int
	PluginsLoaded int
	ConfigPath    string
	BinaryPath    string
	Version       string
	Connected     bool
	LastCheck     time.Time
}

func renderHeader(health HealthState, heartbeat Heartbeat, spinner Spinner, theme Theme, width int, tickInterval time.Duration, apiURL string) string {
	innerWidth := width - 4

	// Status
	statusText := theme.StatusOK.Render("HEALTHY")
	statusIcon := "✅"
	if !health.Connected {
		statusText = theme.StatusFailed.Render("CONNECTING")
		statusIcon = "🔌"
	} else if health.Status != "ok" && health.Status != "" {
		statusText = theme.StatusFailed.Render("DEGRADED")
		statusIcon = "⚠️"
	}

	// Uptime
	uptime := time.Duration(health.UptimeSeconds) * time.Second
	uptimeStr := formatDuration(uptime)

	// Last event
	lastEventStr := "never"
	if !spinner.LastEvent().IsZero() {
		ago := time.Since(spinner.LastEvent()).Round(time.Second)
		lastEventStr = fmt.Sprintf("%s ago", ago)
	}

	// Title line with heartbeat and clock
	heartbeatStr := heartbeat.Render(theme, time.Now(), tickInterval)
	clock := theme.Dim.Render(time.Now().Format("15:04:05"))
	titleText := fmt.Sprintf(" %s DUCTILE WATCH", heartbeatStr)

	// Calculate padding between title and clock
	titleWidth := lipgloss.Width(titleText)
	clockWidth := lipgloss.Width(clock)
	pad := max(innerWidth-titleWidth-clockWidth-4, 1)
	titleLine := titleText + strings.Repeat(" ", pad) + clock + " "

	statsLine := fmt.Sprintf(" %s %s  ⏱ %s  Queue: %d  Plugins: %d",
		statusIcon, statusText,
		uptimeStr,
		health.QueueDepth,
		health.PluginsLoaded,
	)

	metaText := fmt.Sprintf("Config: %s  |  Bin: %s  |  Version: %s  |  API: %s",
		valueOrDash(health.ConfigPath),
		valueOrDash(health.BinaryPath),
		valueOrDash(health.Version),
		valueOrDash(apiURL),
	)
	metaLine := " " + theme.Dim.Render(truncateText(metaText, innerWidth-1))

	// Activity line
	activityLine := fmt.Sprintf(" Last event: %s %s",
		lastEventStr,
		spinner.Render(theme),
	)

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		statsLine,
		metaLine,
		activityLine,
	)

	return theme.Border.Width(innerWidth).Render(content)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

func truncateText(text string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= max {
		return text
	}
	if max <= 3 {
		return string([]rune(text)[:max])
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max-3]) + "..."
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
