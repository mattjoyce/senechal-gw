package live

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

// FocusRegion identifies focusable panels.
type FocusRegion int

const (
	FocusJustNow FocusRegion = iota
	FocusNow
	FocusSoon
	focusCount
)

// workerSlots is the total concurrency capacity to display.
// Matches the default ductile dispatch concurrency.
const workerSlots = 4

// Model is the Live screen state.
type Model struct {
	width, height int
	focus         FocusRegion

	// Just now: recent SSE events
	events      []types.Event
	eventCursor int
	maxEvents   int

	// Now: current jobs
	jobs        []types.Job
	runningJobs []types.Job
	queuedCount int
	failedCount int
	jobCursor   int

	// Soon: scheduled items
	scheduled  []types.SchedulerJob
	soonCursor int
}

func New() Model {
	return Model{
		maxEvents: 30,
	}
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.focus = (m.focus + 1) % focusCount
		case "shift+tab":
			m.focus = (m.focus - 1 + focusCount) % focusCount
		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(1)
		case "enter":
			return m, m.openSelected()
		case "t":
			return m, m.openTree()
		}

	case msgs.SSEEventMsg:
		m.events = append([]types.Event{msg.Event}, m.events...)
		if len(m.events) > m.maxEvents {
			m.events = m.events[:m.maxEvents]
		}

	case msgs.JobsLoadedMsg:
		if msg.Err == nil {
			m.jobs = msg.Data
			m.runningJobs = nil
			m.queuedCount = 0
			m.failedCount = 0
			for _, j := range msg.Data {
				switch j.Status {
				case "running":
					m.runningJobs = append(m.runningJobs, j)
				case "queued":
					m.queuedCount++
				case "failed", "timed_out":
					m.failedCount++
				}
			}
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

	// Calculate panel widths.
	// Each bordered panel adds 2 rows (top + bottom border).
	// Available height must fit: top panels (content + 2) + soon (content + 2).
	borderOverhead := 4 // 2 borders for top panels + 2 for soon
	usable := m.height - borderOverhead
	if usable < 6 {
		usable = 6
	}

	leftW := width*2/5 - 2
	rightW := width*3/5 - 2
	topH := usable * 2 / 3
	if topH < 4 {
		topH = 4
	}
	soonH := usable - topH
	if soonH < 2 {
		soonH = 2
	}

	// Just now panel
	justNow := m.renderJustNow(leftW, topH)

	// Now panel
	now := m.renderNow(rightW, topH)

	// Top row: Just now | Now
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, justNow, now)

	// Soon panel (full width)
	soon := m.renderSoon(width-2, soonH)

	return lipgloss.JoinVertical(lipgloss.Left, topRow, soon)
}

// --- Panel renderers ---

func (m Model) renderJustNow(w, h int) string {
	title := "Just now"
	var lines []string

	if len(m.events) == 0 {
		lines = append(lines, styles.StatusInactive.Render("  (no events yet)"))
	} else {
		visible := h - 3
		if visible < 1 {
			visible = 1
		}
		for i, ev := range m.events {
			if i >= visible {
				break
			}
			data := types.ParseEventData(ev.Data)
			age := styles.RelativeTime(ev.At)
			ageStyled := styles.TimeStyle(ev.At).Render(fmt.Sprintf("%-8s", age))

			evType := truncate(ev.Type, 20)
			subject := data.Plugin
			if data.JobID != "" {
				subject = fmt.Sprintf("%s %s", data.Plugin, truncate(data.JobID, 12))
			}

			line := fmt.Sprintf("  %s  %-20s  %s", ageStyled, evType, subject)
			if m.focus == FocusJustNow && i == m.eventCursor {
				line = styles.SelectedRow.Width(w).Render(line)
			}
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")
	panel := m.panelStyle(FocusJustNow).Width(w).Height(h)
	return panel.Render(fmt.Sprintf(" %s\n%s", styles.TabActive.Render(title), content))
}

func (m Model) renderNow(w, h int) string {
	title := "Now"
	var lines []string

	// Worker slots — show all slots, occupied or idle (spec §6.6)
	for i := range workerSlots {
		if i < len(m.runningJobs) {
			job := m.runningJobs[i]
			elapsed := ""
			if job.StartedAt != nil {
				elapsed = fmt.Sprintf("%ds", int(time.Since(*job.StartedAt).Seconds()))
			}
			line := fmt.Sprintf("  Worker %d  ◉ %-12s  %s › %s  %s",
				i+1,
				truncate(job.JobID, 12),
				truncate(job.Plugin, 10),
				truncate(job.Command, 10),
				elapsed,
			)
			if m.focus == FocusNow && i == m.jobCursor {
				line = styles.SelectedRow.Width(w).Render(line)
			} else {
				line = styles.StatusRunning.Render(line)
			}
			lines = append(lines, line)
		} else {
			line := fmt.Sprintf("  Worker %d  ○ (idle)", i+1)
			if m.focus == FocusNow && i == m.jobCursor {
				line = styles.SelectedRow.Width(w).Render(line)
			} else {
				line = styles.StatusInactive.Render(line)
			}
			lines = append(lines, line)
		}
	}

	// Queue summary band
	lines = append(lines, "")
	summary := fmt.Sprintf("  Queue: %d  Delayed: —  Dead: —",
		m.queuedCount)
	if m.failedCount > 0 {
		summary = fmt.Sprintf("  Queue: %d  Failed: %s  Dead: —",
			m.queuedCount,
			styles.StatusFailed.Render(fmt.Sprintf("%d", m.failedCount)))
	}
	lines = append(lines, styles.SummaryKey.Render(summary))

	content := strings.Join(lines, "\n")
	panel := m.panelStyle(FocusNow).Width(w).Height(h)
	return panel.Render(fmt.Sprintf(" %s\n%s", styles.TabActive.Render(title), content))
}

func (m Model) renderSoon(w, h int) string {
	title := "Soon"
	var lines []string

	if len(m.scheduled) == 0 {
		lines = append(lines, styles.StatusInactive.Render("  (nothing scheduled)"))
	} else {
		visible := h - 2
		if visible < 1 {
			visible = 1
		}
		for i, s := range m.scheduled {
			if i >= visible {
				break
			}
			due := "—"
			if s.NextRunAt != nil {
				due = styles.TimeUntil(*s.NextRunAt)
			}
			line := fmt.Sprintf("  %-8s  %-10s  %-25s  %-15s  %s",
				styles.SoonAccent.Render(fmt.Sprintf("%-8s", due)),
				s.Mode,
				s.ScheduleID,
				s.Plugin,
				s.Command,
			)
			if m.focus == FocusSoon && i == m.soonCursor {
				line = styles.SelectedRow.Width(w).Render(line)
			}
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")
	panel := m.panelStyle(FocusSoon).Width(w).Height(h)
	return panel.Render(fmt.Sprintf(" %s\n%s", styles.SoonAccent.Render(title), content))
}

// --- Helpers ---

func (m Model) panelStyle(region FocusRegion) lipgloss.Style {
	if m.focus == region {
		return styles.PanelFocused
	}
	return styles.PanelUnfocused
}

func (m *Model) moveCursor(delta int) {
	switch m.focus {
	case FocusJustNow:
		m.eventCursor += delta
		if m.eventCursor < 0 {
			m.eventCursor = 0
		}
		if max := len(m.events) - 1; m.eventCursor > max {
			m.eventCursor = max
		}
		if m.eventCursor < 0 {
			m.eventCursor = 0
		}
	case FocusNow:
		m.jobCursor += delta
		if m.jobCursor < 0 {
			m.jobCursor = 0
		}
		// Can navigate all worker slots, not just occupied ones
		if max := workerSlots - 1; m.jobCursor > max {
			m.jobCursor = max
		}
	case FocusSoon:
		m.soonCursor += delta
		if m.soonCursor < 0 {
			m.soonCursor = 0
		}
		if max := len(m.scheduled) - 1; m.soonCursor > max {
			m.soonCursor = max
		}
		if m.soonCursor < 0 {
			m.soonCursor = 0
		}
	}
}

func (m Model) openSelected() tea.Cmd {
	switch m.focus {
	case FocusJustNow:
		if m.eventCursor < len(m.events) {
			ev := m.events[m.eventCursor]
			data := types.ParseEventData(ev.Data)
			if data.JobID != "" {
				return func() tea.Msg {
					return msgs.OpenDetailMsg{Target: types.JobTarget{JobID: data.JobID}}
				}
			}
			return func() tea.Msg {
				return msgs.OpenDetailMsg{Target: types.EventTarget{Event: ev}}
			}
		}
	case FocusNow:
		if m.jobCursor < len(m.runningJobs) {
			jobID := m.runningJobs[m.jobCursor].JobID
			return func() tea.Msg {
				return msgs.OpenDetailMsg{Target: types.JobTarget{JobID: jobID}}
			}
		}
	}
	return nil
}

func (m Model) openTree() tea.Cmd {
	// Find the selected job ID for tree view
	var jobID string
	switch m.focus {
	case FocusJustNow:
		if m.eventCursor < len(m.events) {
			data := types.ParseEventData(m.events[m.eventCursor].Data)
			jobID = data.JobID
		}
	case FocusNow:
		if m.jobCursor < len(m.runningJobs) {
			jobID = m.runningJobs[m.jobCursor].JobID
		}
	}
	if jobID == "" {
		return nil
	}
	// For now, create a single-node tree since we don't have GET /job/{id}/tree yet
	tree := types.TreeNode{
		ID:     jobID,
		Type:   "job",
		Label:  "root",
		Status: "running",
	}
	return func() tea.Msg {
		return msgs.OpenDetailMsg{Target: types.TreeTarget{RootID: jobID, Tree: tree}}
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
