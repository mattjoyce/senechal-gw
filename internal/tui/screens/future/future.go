package future

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/tui/msgs"
	"github.com/mattjoyce/ductile/internal/tui/styles"
	"github.com/mattjoyce/ductile/internal/tui/types"
)

// TimeWindow presets per spec §7.
type TimeWindow int

const (
	WindowAll TimeWindow = iota
	Window5m
	Window30m
	Window2h
	WindowToday
	windowCount
)

var windowLabels = []string{"All", "5m", "30m", "2h", "Today"}

var windowDurations = map[TimeWindow]time.Duration{
	Window5m:  5 * time.Minute,
	Window30m: 30 * time.Minute,
	Window2h:  2 * time.Hour,
}

type Model struct {
	width, height int
	scheduled     []types.SchedulerJob
	filtered      []types.SchedulerJob
	cursor        int
	window        TimeWindow
}

func New() Model { return Model{window: WindowAll} }

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case "tab":
			m.window = (m.window + 1) % windowCount
			m.applyFilter()
		case "shift+tab":
			m.window = (m.window - 1 + windowCount) % windowCount
			m.applyFilter()
		}
	case msgs.SchedulerLoadedMsg:
		if msg.Err == nil {
			m.scheduled = msg.Data
			m.applyFilter()
		}
	}
	return m, nil
}

func (m *Model) applyFilter() {
	if m.window == WindowAll {
		m.filtered = m.scheduled
	} else {
		now := time.Now()
		var horizon time.Time
		if m.window == WindowToday {
			y, mo, d := now.Date()
			horizon = time.Date(y, mo, d+1, 0, 0, 0, 0, now.Location())
		} else if dur, ok := windowDurations[m.window]; ok {
			horizon = now.Add(dur)
		} else {
			m.filtered = m.scheduled
			return
		}

		m.filtered = nil
		for _, s := range m.scheduled {
			if s.NextRunAt != nil && s.NextRunAt.Before(horizon) {
				m.filtered = append(m.filtered, s)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m Model) View(width int) string {
	if width == 0 {
		width = m.width
	}

	var lines []string

	// Window selector
	var windowParts []string
	for i, label := range windowLabels {
		if TimeWindow(i) == m.window {
			windowParts = append(windowParts, styles.TabActive.Render("["+label+"]"))
		} else {
			windowParts = append(windowParts, styles.StatusInactive.Render(" "+label+" "))
		}
	}
	lines = append(lines, "  Window: "+strings.Join(windowParts, " ")+"    (tab to cycle)")
	lines = append(lines, "")

	// Column headers
	hdr := fmt.Sprintf("  %-10s  %-10s  %-25s  %-15s  %-10s  %s",
		"Due in", "Mode", "Schedule ID", "Plugin", "Command", "Status")
	lines = append(lines, styles.SummaryKey.Render(hdr))
	lines = append(lines, styles.StatusInactive.Render("  "+strings.Repeat("─", min(width-6, 90))))

	if len(m.filtered) == 0 {
		lines = append(lines, styles.StatusInactive.Render("  (no scheduled work in this window)"))
	} else {
		for i, s := range m.filtered {
			due := "—"
			if s.NextRunAt != nil {
				due = styles.TimeUntil(*s.NextRunAt)
			}
			statusStyled := styles.StatusStyle(s.Status).Render(s.Status)

			line := fmt.Sprintf("  %-10s  %-10s  %-25s  %-15s  %-10s  %s",
				due, s.Mode, s.ScheduleID, s.Plugin, s.Command, statusStyled)
			if i == m.cursor {
				line = styles.SelectedRow.Width(width).Render(line)
			}
			lines = append(lines, line)
		}
	}

	// Detail pane for selected item
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		s := m.filtered[m.cursor]
		lines = append(lines, "")
		lines = append(lines, styles.StatusInactive.Render("  "+strings.Repeat("─", min(width-6, 90))))
		nextFire := "—"
		if s.NextRunAt != nil {
			nextFire = s.NextRunAt.Format("15:04:05") + " (" + styles.TimeUntil(*s.NextRunAt) + ")"
		}
		lines = append(lines, fmt.Sprintf("  Next fire: %s", styles.SummaryVal.Render(nextFire)))
		lines = append(lines, fmt.Sprintf("  Plugin: %s  Command: %s  Mode: %s",
			styles.SummaryVal.Render(s.Plugin),
			styles.SummaryVal.Render(s.Command),
			styles.SummaryVal.Render(s.Mode)))
		if s.Timezone != "" {
			lines = append(lines, fmt.Sprintf("  Timezone: %s", s.Timezone))
		}
		if s.Reason != "" {
			lines = append(lines, fmt.Sprintf("  Reason: %s", styles.StatusFailed.Render(s.Reason)))
		}
	}

	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}
