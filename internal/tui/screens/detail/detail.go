package detail

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/tui/components/treeview"
	"github.com/mattjoyce/ductile/internal/tui/msgs"
	"github.com/mattjoyce/ductile/internal/tui/styles"
	"github.com/mattjoyce/ductile/internal/tui/types"
)

type Model struct {
	width, height int
	target        types.DetailTarget
	jobDetail     *types.JobDetail
	tree          treeview.Model
	err           error
	breadcrumbs   []string
}

func New() Model {
	return Model{
		tree: treeview.New(),
	}
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.tree.SetSize(w, h)
}

func (m *Model) SetTarget(t types.DetailTarget) {
	m.target = t
	m.jobDetail = nil
	m.err = nil

	// Build breadcrumb
	switch t := t.(type) {
	case types.JobTarget:
		m.breadcrumbs = []string{"Detail", fmt.Sprintf("Job: %s", truncate(t.JobID, 16))}
	case types.EventTarget:
		m.breadcrumbs = []string{"Detail", fmt.Sprintf("Event #%d", t.Event.ID)}
	case types.PluginTarget:
		m.breadcrumbs = []string{"Detail", fmt.Sprintf("Plugin: %s", t.Name)}
	case types.TreeTarget:
		m.breadcrumbs = []string{"Detail", fmt.Sprintf("Tree: %s", truncate(t.RootID, 16))}
		m.tree.SetTree(t.Tree)
	default:
		m.breadcrumbs = []string{"Detail"}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return msgs.NavigateBackMsg{} }
		}

		// If showing a tree, forward keys to tree
		if _, ok := m.target.(types.TreeTarget); ok {
			var cmd tea.Cmd
			m.tree, cmd = m.tree.Update(msg)
			return m, cmd
		}

	case msgs.JobDetailLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
		} else {
			m.jobDetail = &msg.Data
		}
	}
	return m, nil
}

func (m Model) View(width int) string {
	if width == 0 {
		width = m.width
	}

	if m.target == nil {
		return styles.StatusInactive.Render("  (no item selected — press enter from another screen)")
	}

	var lines []string

	// Breadcrumb trail
	crumb := strings.Join(m.breadcrumbs, " > ")
	lines = append(lines, styles.SummaryKey.Render("  "+crumb))
	lines = append(lines, styles.StatusInactive.Render("  "+strings.Repeat("─", min(width-6, 80))))

	switch t := m.target.(type) {
	case types.JobTarget:
		lines = append(lines, m.renderJobDetail(t, width)...)
	case types.EventTarget:
		lines = append(lines, m.renderEventDetail(t, width)...)
	case types.PluginTarget:
		lines = append(lines, m.renderPluginDetail(t, width)...)
	case types.TreeTarget:
		lines = append(lines, "")
		lines = append(lines, m.tree.View())
	default:
		lines = append(lines, "  Unknown detail target")
	}

	if m.err != nil {
		lines = append(lines, "")
		lines = append(lines, styles.StatusFailed.Render(fmt.Sprintf("  Error: %s", m.err)))
	}

	lines = append(lines, "")
	lines = append(lines, styles.StatusInactive.Render("  esc = back"))

	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func (m Model) renderJobDetail(t types.JobTarget, width int) []string {
	var lines []string

	if m.jobDetail == nil {
		lines = append(lines, styles.StatusInactive.Render("  Loading..."))
		return lines
	}

	d := m.jobDetail
	lines = append(lines, fmt.Sprintf("  Job ID:    %s", styles.SummaryVal.Render(d.JobID)))
	lines = append(lines, fmt.Sprintf("  Status:    %s", styles.StatusStyle(d.Status).Render(d.Status)))
	lines = append(lines, fmt.Sprintf("  Plugin:    %s", styles.SummaryVal.Render(d.Plugin)))
	lines = append(lines, fmt.Sprintf("  Command:   %s", styles.SummaryVal.Render(d.Command)))

	if d.StartedAt != nil {
		lines = append(lines, fmt.Sprintf("  Started:   %s (%s)", d.StartedAt.Format("15:04:05"), styles.RelativeTime(*d.StartedAt)))
	}
	if d.CompletedAt != nil {
		lines = append(lines, fmt.Sprintf("  Completed: %s (%s)", d.CompletedAt.Format("15:04:05"), styles.RelativeTime(*d.CompletedAt)))
	}
	if d.StartedAt != nil && d.CompletedAt != nil {
		dur := d.CompletedAt.Sub(*d.StartedAt)
		lines = append(lines, fmt.Sprintf("  Duration:  %s", styles.SummaryVal.Render(dur.String())))
	}

	if len(d.Result) > 0 && string(d.Result) != "null" {
		lines = append(lines, "")
		lines = append(lines, styles.SummaryKey.Render("  Result:"))
		var pretty json.RawMessage
		if json.Unmarshal(d.Result, &pretty) == nil {
			formatted, err := json.MarshalIndent(pretty, "    ", "  ")
			if err == nil {
				for _, rl := range strings.Split(string(formatted), "\n") {
					lines = append(lines, "    "+rl)
				}
			} else {
				lines = append(lines, "    "+string(d.Result))
			}
		}
	}

	return lines
}

func (m Model) renderEventDetail(t types.EventTarget, _ int) []string {
	ev := t.Event
	data := types.ParseEventData(ev.Data)

	var lines []string
	lines = append(lines, fmt.Sprintf("  Type:    %s", styles.SummaryVal.Render(ev.Type)))
	lines = append(lines, fmt.Sprintf("  Time:    %s (%s)", ev.At.Format("15:04:05"), styles.RelativeTime(ev.At)))
	if data.JobID != "" {
		lines = append(lines, fmt.Sprintf("  Job ID:  %s", data.JobID))
	}
	if data.Plugin != "" {
		lines = append(lines, fmt.Sprintf("  Plugin:  %s", data.Plugin))
	}
	if data.Command != "" {
		lines = append(lines, fmt.Sprintf("  Command: %s", data.Command))
	}
	if data.Pipeline != "" {
		lines = append(lines, fmt.Sprintf("  Pipeline: %s", data.Pipeline))
	}
	if data.Error != "" {
		lines = append(lines, fmt.Sprintf("  Error:   %s", styles.StatusFailed.Render(data.Error)))
	}

	// Raw data
	lines = append(lines, "")
	lines = append(lines, styles.SummaryKey.Render("  Raw data:"))
	var pretty json.RawMessage
	if json.Unmarshal(ev.Data, &pretty) == nil {
		formatted, err := json.MarshalIndent(pretty, "    ", "  ")
		if err == nil {
			for _, rl := range strings.Split(string(formatted), "\n") {
				lines = append(lines, "    "+rl)
			}
		}
	}

	return lines
}

func (m Model) renderPluginDetail(t types.PluginTarget, _ int) []string {
	var lines []string
	lines = append(lines, fmt.Sprintf("  Name: %s", styles.SummaryVal.Render(t.Name)))
	lines = append(lines, styles.StatusInactive.Render("  (plugin detail view)"))
	return lines
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
