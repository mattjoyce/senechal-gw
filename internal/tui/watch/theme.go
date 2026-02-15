// Package watch implements the ductile system watch (Overwatch) TUI.
// See docs/TUI_WATCH_DESIGN.md for the full specification.
package watch

import "github.com/charmbracelet/lipgloss"

// Theme centralizes all styling for the watch TUI.
// Even with a single default theme, this keeps all colors in one place
// and makes future theme support trivial.
type Theme struct {
	// Status colors
	StatusOK      lipgloss.Style
	StatusRunning lipgloss.Style
	StatusFailed  lipgloss.Style
	StatusQueued  lipgloss.Style
	StatusDead    lipgloss.Style

	// UI elements
	Border    lipgloss.Style
	Title     lipgloss.Style
	Header    lipgloss.Style
	Dim       lipgloss.Style
	Highlight lipgloss.Style

	// Indicators
	TickerActive   lipgloss.Style
	TickerInactive lipgloss.Style
	Progress       lipgloss.Style
}

func NewDefaultTheme() Theme {
	purple := lipgloss.Color("#874BFD")

	return Theme{
		StatusOK:      lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")),
		StatusRunning: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")),
		StatusFailed:  lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")),
		StatusQueued:  lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
		StatusDead:    lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")),

		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(purple),
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Padding(0, 1),
		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#61AFEF")),
		Dim:       lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")),
		Highlight: lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B")),

		TickerActive:   lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")),
		TickerInactive: lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")),
		Progress:       lipgloss.NewStyle().Foreground(lipgloss.Color("#61AFEF")),
	}
}
