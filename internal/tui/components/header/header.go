package header

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/tui/components/activity"
	"github.com/mattjoyce/ductile/internal/tui/styles"
	"github.com/mattjoyce/ductile/internal/tui/types"
)

// Model holds header state.
type Model struct {
	Health    types.RuntimeHealth
	Connected bool
	Heartbeat activity.Heartbeat
	Dots      activity.Dots
	ApiURL    string

	// Ticker rotates every second to prove TUI is alive.
	tickerFrame int
}

var tickerFrames = []string{"⟲", "⟳"}

func New(apiURL string) Model {
	return Model{
		ApiURL:    apiURL,
		Heartbeat: activity.NewHeartbeat(),
		Dots:      activity.NewDots(),
	}
}

func (m *Model) Tick() {
	m.tickerFrame = (m.tickerFrame + 1) % len(tickerFrames)
	m.Dots.Decay()
}

// Height returns the number of terminal rows the header occupies (content + border).
func (m Model) Height() int {
	return 4 // 2 content lines + 2 border lines (top/bottom)
}

// View renders the two-line header inside a bordered box per spec §6.4.
//
// Line 1: ⟲ ♥ DUCTILE WATCH  ✅ HEALTHY  ⏱ 3h 33m  Queue: 0  Plugins: 23  Last: 5s ●●●○○   22:23:10
// Line 2: Config: ~/.config/ductile/  |  Bin: ~/.local/bin/ductile  |  Version: 1.0.0  |  API: http://localhost:8081
func (m Model) View(width int) string {
	now := time.Now()
	innerWidth := width - 4 // account for border + padding

	// --- Line 1: operational summary ---
	var parts []string

	// Ticker (local-tick driven, proves TUI isn't frozen)
	parts = append(parts, tickerFrames[m.tickerFrame])

	// Heartbeat (SSE-driven, proves backend is pulsing)
	parts = append(parts, m.Heartbeat.Render(now, time.Minute))

	// Title
	parts = append(parts, styles.TabActive.Render("DUCTILE WATCH"))

	// Health icon + status
	switch {
	case !m.Connected:
		parts = append(parts, "🔌 "+styles.StatusWarning.Render("CONNECTING"))
	case m.Health.Status == "ok":
		parts = append(parts, "✅ "+styles.StatusHealthy.Render("HEALTHY"))
	default:
		parts = append(parts, "⚠️ "+styles.StatusWarning.Render(strings.ToUpper(m.Health.Status)))
	}

	// Uptime
	uptime := time.Duration(m.Health.UptimeSeconds) * time.Second
	parts = append(parts, "⏱ "+formatUptime(uptime))

	// Queue depth (coloured by state)
	qStyle := styles.StatusHealthy
	if m.Health.QueueDepth > 0 {
		qStyle = styles.StatusQueued
	}
	parts = append(parts, "Queue: "+qStyle.Render(fmt.Sprintf("%d", m.Health.QueueDepth)))

	// Plugins loaded
	parts = append(parts, fmt.Sprintf("Plugins: %d", m.Health.PluginsLoaded))

	// Last event age + activity dots
	lastAge := "—"
	if !m.Dots.LastEvent().IsZero() {
		ago := time.Since(m.Dots.LastEvent()).Round(time.Second)
		if ago < time.Minute {
			lastAge = fmt.Sprintf("%ds", int(ago.Seconds()))
		} else if ago < time.Hour {
			lastAge = fmt.Sprintf("%dm", int(ago.Minutes()))
		} else {
			lastAge = fmt.Sprintf("%dh", int(ago.Hours()))
		}
	}
	parts = append(parts, fmt.Sprintf("Last: %s %s", lastAge, m.Dots.Render()))

	// Clock (right-aligned)
	clock := now.Format("15:04:05")

	line1Content := strings.Join(parts, "  ")
	contentWidth := lipgloss.Width(line1Content)
	clockWidth := lipgloss.Width(clock)
	pad := innerWidth - contentWidth - clockWidth
	if pad < 1 {
		pad = 1
	}
	line1 := line1Content + strings.Repeat(" ", pad) + clock

	// --- Line 2: static metadata (dimmed) ---
	meta := fmt.Sprintf("Config: %s  |  Bin: %s  |  Version: %s  |  API: %s",
		valueOrDash(m.Health.ConfigPath),
		valueOrDash(m.Health.BinaryPath),
		valueOrDash(m.Health.Version),
		valueOrDash(m.ApiURL),
	)
	if lipgloss.Width(meta) > innerWidth {
		runes := []rune(meta)
		if len(runes) > innerWidth-3 {
			meta = string(runes[:innerWidth-3]) + "..."
		}
	}
	line2 := styles.HeaderDim.Render(meta)

	content := lipgloss.JoinVertical(lipgloss.Left, line1, line2)

	return styles.HeaderBorder.Width(innerWidth).Render(content)
}

func formatUptime(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func valueOrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
