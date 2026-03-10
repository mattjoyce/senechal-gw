package structure

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/tui/msgs"
	"github.com/mattjoyce/ductile/internal/tui/styles"
	"github.com/mattjoyce/ductile/internal/tui/types"
)

type Model struct {
	width, height int
	plugins       []types.PluginSummary
	scheduled     []types.SchedulerJob
	cursor        int
}

func New() Model { return Model{} }

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
			total := len(m.plugins) + len(m.scheduled)
			if m.cursor < total-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor < len(m.plugins) {
				p := m.plugins[m.cursor]
				return m, func() tea.Msg {
					return msgs.OpenDetailMsg{Target: types.PluginTarget{Name: p.Name}}
				}
			}
		}
	case msgs.PluginsLoadedMsg:
		if msg.Err == nil {
			m.plugins = msg.Data
		}
	case msgs.SchedulerLoadedMsg:
		if msg.Err == nil {
			m.scheduled = msg.Data
		}
	}
	return m, nil
}

func (m Model) View(width int) string {
	if width == 0 {
		width = m.width
	}

	var lines []string

	// Plugins section
	lines = append(lines, styles.TabActive.Render("  Plugins"))
	lines = append(lines, styles.StatusInactive.Render(strings.Repeat("─", min(width-4, 80))))

	if len(m.plugins) == 0 {
		lines = append(lines, styles.StatusInactive.Render("  (no plugins loaded)"))
	} else {
		for i, p := range m.plugins {
			cmds := strings.Join(p.Commands, ", ")
			line := fmt.Sprintf("  %-20s  v%s  [%s]", p.Name, p.Version, cmds)
			if i == m.cursor {
				line = styles.SelectedRow.Width(width).Render(line)
			}
			lines = append(lines, line)
		}
	}

	lines = append(lines, "")

	// Schedules section
	lines = append(lines, styles.TabActive.Render("  Schedules"))
	lines = append(lines, styles.StatusInactive.Render(strings.Repeat("─", min(width-4, 80))))

	if len(m.scheduled) == 0 {
		lines = append(lines, styles.StatusInactive.Render("  (no schedules configured)"))
	} else {
		for i, s := range m.scheduled {
			due := "—"
			if s.NextRunAt != nil {
				due = styles.TimeUntil(*s.NextRunAt)
			}
			line := fmt.Sprintf("  %-20s  %-10s  %-10s  %s  Next: %s",
				truncate(s.ScheduleID, 20), s.Plugin, s.Mode, s.Command, due)
			idx := len(m.plugins) + i
			if idx == m.cursor {
				line = styles.SelectedRow.Width(width).Render(line)
			}
			lines = append(lines, line)
		}
	}

	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
