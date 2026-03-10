package past

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/tui/msgs"
	"github.com/mattjoyce/ductile/internal/tui/styles"
	"github.com/mattjoyce/ductile/internal/tui/types"
)

// Layer tracks the drill-down depth.
type Layer int

const (
	LayerAggregate Layer = iota // surface summary
	LayerGroup                  // plugin/error group detail
)

// TimeWindow for the Past screen.
type TimeWindow int

const (
	Window5m TimeWindow = iota
	Window15m
	Window60m
	Window24h
	windowCount
)

var windowLabels = []string{"5m", "15m", "60m", "24h"}
var windowDurations = []time.Duration{
	5 * time.Minute,
	15 * time.Minute,
	60 * time.Minute,
	24 * time.Hour,
}

// WindowDuration returns the duration for the current window selection.
// Exported so the app can use it when fetching job-logs with a `from` param.
func WindowDuration(w TimeWindow) time.Duration {
	if int(w) < len(windowDurations) {
		return windowDurations[w]
	}
	return time.Hour
}

// groupType identifies what we drilled into.
type groupType int

const (
	groupPlugin groupType = iota
	groupError
)

type drillGroup struct {
	kind  groupType
	key   string // plugin name or error signature
	label string
}

type Model struct {
	width, height int
	logs          []types.JobLog
	total         int
	Window        TimeWindow
	layer         Layer
	cursor        int
	group         *drillGroup // non-nil when in layer 2

	// Computed aggregates
	succeeded   int
	failed      int
	timedOut    int
	avgDuration time.Duration
	p95Duration time.Duration
	pluginStats []pluginStat
	errorGroups []errorGroup

	// WindowChanged signals app to re-fetch with new time range
	WindowChanged bool
}

type pluginStat struct {
	name string
	ok   int
	fail int
	avg  time.Duration
}

type errorGroup struct {
	signature string
	count     int
}

func New() Model { return Model{Window: Window60m} }

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
			m.cursor++
			m.clampCursor()
		case "esc":
			if m.layer > LayerAggregate {
				m.layer = LayerAggregate
				m.group = nil
				m.cursor = 0
				return m, nil
			}
		case "enter":
			return m.handleEnter()
		case "tab":
			m.Window = (m.Window + 1) % windowCount
			m.WindowChanged = true
			m.recompute()
		case "shift+tab":
			m.Window = (m.Window - 1 + windowCount) % windowCount
			m.WindowChanged = true
			m.recompute()
		}
	case msgs.JobLogsLoadedMsg:
		if msg.Err == nil {
			m.logs = msg.Data
			m.total = msg.Total
			m.recompute()
		}
	}
	return m, nil
}

func (m Model) View(width int) string {
	if width == 0 {
		width = m.width
	}

	switch m.layer {
	case LayerGroup:
		return m.viewGroup(width)
	default:
		return m.viewAggregate(width)
	}
}

func (m Model) viewAggregate(width int) string {
	var lines []string

	// Window selector
	var windowParts []string
	for i, label := range windowLabels {
		if TimeWindow(i) == m.Window {
			windowParts = append(windowParts, styles.TabActive.Render("["+label+"]"))
		} else {
			windowParts = append(windowParts, styles.StatusInactive.Render(" "+label+" "))
		}
	}
	lines = append(lines, "  Window: "+strings.Join(windowParts, " ")+"    (tab to cycle)")
	lines = append(lines, "")

	// Summary stats
	lines = append(lines, fmt.Sprintf("  Completed: %s   Failed: %s   Timed out: %s",
		styles.StatusHealthy.Render(fmt.Sprintf("%d", m.succeeded)),
		styles.StatusFailed.Render(fmt.Sprintf("%d", m.failed)),
		styles.StatusWarning.Render(fmt.Sprintf("%d", m.timedOut)),
	))
	lines = append(lines, fmt.Sprintf("  Avg duration: %s   P95: %s",
		styles.SummaryVal.Render(m.avgDuration.Round(time.Millisecond).String()),
		styles.SummaryVal.Render(m.p95Duration.Round(time.Millisecond).String()),
	))
	lines = append(lines, "")

	// By plugin
	lines = append(lines, styles.SummaryKey.Render("  ─ By plugin ─────────────────────────"))
	if len(m.pluginStats) == 0 {
		lines = append(lines, styles.StatusInactive.Render("    (no jobs in this window)"))
	}
	for i, ps := range m.pluginStats {
		line := fmt.Sprintf("    %-15s  %s ok  %s fail  avg %s",
			ps.name,
			styles.StatusHealthy.Render(fmt.Sprintf("%d", ps.ok)),
			styles.StatusFailed.Render(fmt.Sprintf("%d", ps.fail)),
			ps.avg.Round(time.Millisecond).String(),
		)
		if i == m.cursor {
			line = styles.SelectedRow.Width(width).Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")

	// Error signatures
	offset := len(m.pluginStats)
	lines = append(lines, styles.SummaryKey.Render("  ─ Error signatures ──────────────────"))
	if len(m.errorGroups) == 0 {
		lines = append(lines, styles.StatusInactive.Render("    (none)"))
	} else {
		for i, eg := range m.errorGroups {
			line := fmt.Sprintf("    %-40s  (%d occurrences)",
				truncate(eg.signature, 40),
				eg.count,
			)
			if i+offset == m.cursor {
				line = styles.SelectedRow.Width(width).Render(line)
			}
			lines = append(lines, line)
		}
	}

	lines = append(lines, "")
	lines = append(lines, styles.StatusInactive.Render("  enter = drill down  esc = back  tab = window"))

	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func (m Model) viewGroup(width int) string {
	var lines []string

	if m.group == nil {
		return styles.StatusInactive.Render("  (no group selected)")
	}

	lines = append(lines, styles.TabActive.Render(fmt.Sprintf("  %s", m.group.label)))
	lines = append(lines, styles.StatusInactive.Render("  "+strings.Repeat("─", min(width-6, 80))))

	// Filter logs to this group
	filtered := m.filterByGroup()

	if len(filtered) == 0 {
		lines = append(lines, styles.StatusInactive.Render("  (no matching jobs)"))
	} else {
		hdr := fmt.Sprintf("  %-12s  %-10s  %-10s  %-10s  %-8s  %s",
			"Job ID", "Plugin", "Command", "Status", "Attempt", "Completed")
		lines = append(lines, styles.SummaryKey.Render(hdr))

		for i, log := range filtered {
			completed := "—"
			if !log.CompletedAt.IsZero() {
				completed = styles.RelativeTime(log.CompletedAt)
			}
			statusStyled := styles.StatusStyle(log.Status).Render(fmt.Sprintf("%-10s", log.Status))

			line := fmt.Sprintf("  %-12s  %-10s  %-10s  %s  %-8d  %s",
				truncate(log.JobID, 12),
				truncate(log.Plugin, 10),
				truncate(log.Command, 10),
				statusStyled,
				log.Attempt,
				completed,
			)
			if i == m.cursor {
				line = styles.SelectedRow.Width(width).Render(line)
			}
			lines = append(lines, line)
		}
	}

	lines = append(lines, "")
	lines = append(lines, styles.StatusInactive.Render("  enter = job detail  esc = back"))

	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

// --- Internal ---

func (m *Model) recompute() {
	m.succeeded = 0
	m.failed = 0
	m.timedOut = 0
	m.pluginStats = nil
	m.errorGroups = nil

	// All logs are already filtered by time window via the API fetch,
	// so we compute aggregates over all logs we have.
	pluginMap := map[string]*pluginStat{}
	errorMap := map[string]int{}
	var durations []time.Duration

	for _, log := range m.logs {
		ps, ok := pluginMap[log.Plugin]
		if !ok {
			ps = &pluginStat{name: log.Plugin}
			pluginMap[log.Plugin] = ps
		}

		dur := time.Duration(0)
		if !log.CompletedAt.IsZero() {
			dur = log.CompletedAt.Sub(log.CreatedAt)
		}

		switch log.Status {
		case "succeeded":
			m.succeeded++
			ps.ok++
			if dur > 0 {
				durations = append(durations, dur)
			}
		case "failed":
			m.failed++
			ps.fail++
			if log.LastError != nil && *log.LastError != "" {
				errorMap[*log.LastError]++
			}
		case "timed_out":
			m.timedOut++
			ps.fail++
			errorMap["timeout"]++
		}

		if dur > 0 {
			ps.avg += dur
		}
	}

	// Compute averages
	if len(durations) > 0 {
		var total time.Duration
		for _, d := range durations {
			total += d
		}
		m.avgDuration = total / time.Duration(len(durations))

		// P95
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		p95idx := int(float64(len(durations)) * 0.95)
		if p95idx >= len(durations) {
			p95idx = len(durations) - 1
		}
		m.p95Duration = durations[p95idx]
	} else {
		m.avgDuration = 0
		m.p95Duration = 0
	}

	// Build plugin stats
	for _, ps := range pluginMap {
		total := ps.ok + ps.fail
		if total > 0 && ps.avg > 0 {
			ps.avg = ps.avg / time.Duration(total)
		}
		m.pluginStats = append(m.pluginStats, *ps)
	}
	sort.Slice(m.pluginStats, func(i, j int) bool {
		return (m.pluginStats[i].ok + m.pluginStats[i].fail) > (m.pluginStats[j].ok + m.pluginStats[j].fail)
	})

	// Build error groups
	for sig, count := range errorMap {
		m.errorGroups = append(m.errorGroups, errorGroup{signature: sig, count: count})
	}
	sort.Slice(m.errorGroups, func(i, j int) bool {
		return m.errorGroups[i].count > m.errorGroups[j].count
	})

	m.clampCursor()
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	switch m.layer {
	case LayerAggregate:
		if m.cursor < len(m.pluginStats) {
			ps := m.pluginStats[m.cursor]
			m.layer = LayerGroup
			m.group = &drillGroup{kind: groupPlugin, key: ps.name, label: fmt.Sprintf("Plugin: %s", ps.name)}
			m.cursor = 0
			return m, nil
		}
		offset := len(m.pluginStats)
		if m.cursor-offset >= 0 && m.cursor-offset < len(m.errorGroups) {
			eg := m.errorGroups[m.cursor-offset]
			m.layer = LayerGroup
			m.group = &drillGroup{kind: groupError, key: eg.signature, label: fmt.Sprintf("Error: %s", truncate(eg.signature, 50))}
			m.cursor = 0
			return m, nil
		}
	case LayerGroup:
		filtered := m.filterByGroup()
		if m.cursor < len(filtered) {
			log := filtered[m.cursor]
			return m, func() tea.Msg {
				return msgs.OpenDetailMsg{Target: types.JobTarget{JobID: log.JobID}}
			}
		}
	}
	return m, nil
}

func (m Model) filterByGroup() []types.JobLog {
	if m.group == nil {
		return m.logs
	}
	var filtered []types.JobLog
	for _, log := range m.logs {
		switch m.group.kind {
		case groupPlugin:
			if log.Plugin == m.group.key {
				filtered = append(filtered, log)
			}
		case groupError:
			if log.LastError != nil && *log.LastError == m.group.key {
				filtered = append(filtered, log)
			}
			if m.group.key == "timeout" && log.Status == "timed_out" {
				filtered = append(filtered, log)
			}
		}
	}
	return filtered
}

func (m *Model) clampCursor() {
	maxItems := m.itemCount()
	if m.cursor >= maxItems {
		m.cursor = max(0, maxItems-1)
	}
}

func (m Model) itemCount() int {
	switch m.layer {
	case LayerAggregate:
		return len(m.pluginStats) + len(m.errorGroups)
	case LayerGroup:
		return len(m.filterByGroup())
	default:
		return 0
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
