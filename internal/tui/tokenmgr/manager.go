package tokenmgr

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/plugin"
)

var (
	titleStyle        = lipgloss.NewStyle().MarginLeft(2)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
)

type item struct {
	scope    string
	desc     string
	selected bool
}

func (i item) Title() string {
	check := "[ ]"
	if i.selected {
		check = "[x]"
	}
	return fmt.Sprintf("%s %s", check, i.scope)
}
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.scope }

type model struct {
	list     list.Model
	choice   string
	quitting bool
	done     bool
	scopes   []string
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case " ": // Space to toggle
			i, ok := m.list.SelectedItem().(item)
			if ok {
				i.selected = !i.selected
				m.list.SetItem(m.list.Index(), i)
			}
			return m, nil

		case "enter":
			m.done = true
			var selected []string
			for _, li := range m.list.Items() {
				if it, ok := li.(item); ok && it.selected {
					selected = append(selected, it.scope)
				}
			}
			m.scopes = selected
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return quitTextStyle.Render("Cancelled.")
	}
	if m.done {
		return quitTextStyle.Render(fmt.Sprintf("Selected scopes: %s", strings.Join(m.scopes, ", ")))
	}
	return "\n" + m.list.View()
}

func New(registry *plugin.Registry) *model {
	var items []list.Item

	// Core Scopes
	coreScopes := []struct {
		scope string
		desc  string
	}{
		{"*", "Full administrative access (all scopes)"},
		{"jobs:ro", "Read-only access to job execution history"},
		{"jobs:rw", "Full access to jobs (trigger and view)"},
		{"plugin:ro", "Read access to plugin metadata and read-only commands"},
		{"plugin:rw", "Full access to invoke any plugin command"},
		{"events:ro", "Access to the real-time event stream (SSE)"},
	}

	for _, s := range coreScopes {
		items = append(items, item{scope: s.scope, desc: s.desc})
	}

	// Plugin-specific Scopes
	for _, p := range registry.All() {
		desc := p.Description
		if desc == "" {
			desc = fmt.Sprintf("Access to the %s plugin", p.Name)
		}
		items = append(items, item{
			scope: fmt.Sprintf("plugin:%s", p.Name),
			desc:  desc,
		})
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select Scopes (Space to toggle, Enter to confirm)"
	l.Styles.Title = titleStyle
	l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = helpStyle

	return &model{list: l}
}

func (m *model) GetSelectedScopes() []string {
	return m.scopes
}
