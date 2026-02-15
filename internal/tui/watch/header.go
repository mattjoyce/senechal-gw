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
	Connected     bool
	LastCheck     time.Time
}

func renderHeader(health HealthState, ticker Ticker, spinner Spinner, theme Theme, width int) string {
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

	// Title line with ticker and clock
	tickerStr := theme.Highlight.Render(ticker.Current())
	clock := theme.Dim.Render(time.Now().Format("15:04:05"))
	titleText := fmt.Sprintf(" DUCTILE WATCH %s", tickerStr)

	// Calculate padding between title and clock
	titleWidth := lipgloss.Width(titleText)
	clockWidth := lipgloss.Width(clock)
	pad := innerWidth - titleWidth - clockWidth - 4
	if pad < 1 {
		pad = 1
	}
	titleLine := titleText + strings.Repeat(" ", pad) + clock + " "

	// Stats line
	statsLine := fmt.Sprintf(" %s %s  ⏱ %s  Queue: %d  Plugins: %d",
		statusIcon, statusText,
		uptimeStr,
		health.QueueDepth,
		health.PluginsLoaded,
	)

	// Activity line
	activityLine := fmt.Sprintf(" Last event: %s %s",
		lastEventStr,
		spinner.Render(theme),
	)

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		statsLine,
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
