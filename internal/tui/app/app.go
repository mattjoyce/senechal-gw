package app

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/tui/client"
	"github.com/mattjoyce/ductile/internal/tui/components/header"
	"github.com/mattjoyce/ductile/internal/tui/msgs"
	"github.com/mattjoyce/ductile/internal/tui/screens/detail"
	"github.com/mattjoyce/ductile/internal/tui/screens/future"
	"github.com/mattjoyce/ductile/internal/tui/screens/live"
	"github.com/mattjoyce/ductile/internal/tui/screens/past"
	"github.com/mattjoyce/ductile/internal/tui/screens/structure"
	"github.com/mattjoyce/ductile/internal/tui/styles"
	"github.com/mattjoyce/ductile/internal/tui/types"
)

// Tab identifies a top-level screen.
type Tab int

const (
	TabLive Tab = iota
	TabFuture
	TabPast
	TabStructure
	TabDetail
)

var tabNames = []string{"Live", "Future", "Past", "Structure", "Detail"}

// Model is the root Bubble Tea model.
type Model struct {
	width, height int
	activeTab     Tab
	prevTab       Tab // for navigating back from detail

	header    header.Model
	live      live.Model
	future    future.Model
	past      past.Model
	structure structure.Model
	detail    detail.Model

	client *client.Client

	// SSE
	sseEvents chan types.Event
	frozen    bool

	showHelp bool
	lastErr  string
}

// New creates the root model.
func New(apiURL, apiKey string) Model {
	c := client.New(apiURL, apiKey)
	return Model{
		activeTab: TabLive,
		header:    header.New(apiURL),
		live:      live.New(),
		future:    future.New(),
		past:      past.New(),
		structure: structure.New(),
		detail:    detail.New(),
		client:    c,
		sseEvents: make(chan types.Event, 100),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.fetchHealth(),
		m.fetchSchedulerJobs(),
		m.fetchPlugins(),
		m.subscribeToEvents(),
		m.receiveNextEvent(),
		tea.Tick(time.Second, func(time.Time) tea.Msg { return msgs.LiveTickMsg{} }),
		tea.Tick(2*time.Second, func(time.Time) tea.Msg { return msgs.FutureTickMsg{} }),
		tea.Tick(5*time.Second, func(time.Time) tea.Msg { return msgs.PastTickMsg{} }),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Help overlay consumes esc/?
		if m.showHelp {
			if msg.String() == "?" || msg.String() == "esc" || msg.String() == "q" {
				m.showHelp = false
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = true
			return m, nil
		case "1":
			m.activeTab = TabLive
		case "2":
			m.activeTab = TabFuture
		case "3":
			m.activeTab = TabPast
		case "4":
			m.activeTab = TabStructure
		case "5":
			m.activeTab = TabDetail
		case "esc":
			if m.activeTab == TabDetail {
				m.activeTab = m.prevTab
				return m, nil
			}
			return m.updateActiveScreen(msg)
		case " ":
			m.frozen = !m.frozen
			return m, nil
		case "r":
			return m, tea.Batch(
				m.fetchHealth(),
				m.fetchSchedulerJobs(),
				m.fetchPlugins(),
				m.fetchJobs(),
				m.fetchJobLogs(),
			)
		default:
			return m.updateActiveScreen(msg)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Header (4 rows: 2 content + 2 border) + tab bar (1) + freeze/error bars (up to 2)
		chrome := m.header.Height() + 1 + 2
		screenH := msg.Height - chrome
		if screenH < 4 {
			screenH = 4
		}
		m.live.SetSize(msg.Width, screenH)
		m.future.SetSize(msg.Width, screenH)
		m.past.SetSize(msg.Width, screenH)
		m.structure.SetSize(msg.Width, screenH)
		m.detail.SetSize(msg.Width, screenH)
		return m, nil

	// --- Tick messages ---
	case msgs.LiveTickMsg:
		m.header.Tick()
		if !m.frozen {
			cmds = append(cmds, m.fetchHealth())
			cmds = append(cmds, m.fetchJobs())
		}
		cmds = append(cmds, tea.Tick(time.Second, func(time.Time) tea.Msg { return msgs.LiveTickMsg{} }))
		return m.forwardAndBatch(msg, cmds)

	case msgs.FutureTickMsg:
		if !m.frozen {
			cmds = append(cmds, m.fetchSchedulerJobs())
		}
		cmds = append(cmds, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return msgs.FutureTickMsg{} }))
		return m.forwardAndBatch(msg, cmds)

	case msgs.PastTickMsg:
		if !m.frozen {
			cmds = append(cmds, m.fetchJobLogs())
		}
		cmds = append(cmds, tea.Tick(5*time.Second, func(time.Time) tea.Msg { return msgs.PastTickMsg{} }))
		model, cmd := m.forwardAndBatch(msg, cmds)
		// Check if past screen changed its window — re-fetch immediately
		if app, ok := model.(Model); ok && app.past.WindowChanged {
			app.past.WindowChanged = false
			return app, tea.Batch(cmd, app.fetchJobLogs())
		}
		return model, cmd

	// --- SSE messages ---
	case msgs.SSEEventMsg:
		m.header.Dots.OnEvent()
		if msg.Event.Type == "scheduler.tick" {
			m.header.Heartbeat.OnTick()
		}
		m.header.Connected = true
		m.lastErr = ""

		// SSE event triggers immediate queue refresh
		if !m.frozen {
			cmds = append(cmds, m.fetchJobs())
		}
		cmds = append(cmds, m.receiveNextEvent())
		return m.forwardAndBatch(msg, cmds)

	case msgs.SSEDisconnectedMsg:
		m.header.Connected = false
		m.lastErr = "SSE disconnected, reconnecting..."
		cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return msgs.SSEReconnectMsg{} }))
		return m, tea.Batch(cmds...)

	case msgs.SSEReconnectMsg:
		cmds = append(cmds, m.subscribeToEvents())
		cmds = append(cmds, m.fetchJobs()) // sync gap fill
		return m, tea.Batch(cmds...)

	// --- Data loaded messages ---
	case msgs.HealthLoadedMsg:
		if msg.Err != nil {
			m.lastErr = msg.Err.Error()
		} else {
			m.header.Health = msg.Data
			m.header.Connected = true
			m.lastErr = ""
		}
		return m.forwardAndBatch(msg, nil)

	case msgs.OpenDetailMsg:
		m.prevTab = m.activeTab
		m.activeTab = TabDetail
		m.detail.SetTarget(msg.Target)
		if jt, ok := msg.Target.(types.JobTarget); ok {
			cmds = append(cmds, m.fetchJobDetail(jt.JobID))
		}
		return m, tea.Batch(cmds...)

	case msgs.NavigateBackMsg:
		if m.activeTab == TabDetail {
			m.activeTab = m.prevTab
		}
		return m, nil

	default:
		// Forward ALL unhandled messages to active screen
		return m.updateActiveScreen(msg)
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	if m.showHelp {
		return m.renderHelp()
	}

	// Header
	hdr := m.header.View(m.width)

	// Tab bar
	tabs := m.renderTabBar()

	// Active screen
	var screen string
	switch m.activeTab {
	case TabLive:
		screen = m.live.View(m.width)
	case TabFuture:
		screen = m.future.View(m.width)
	case TabPast:
		screen = m.past.View(m.width)
	case TabStructure:
		screen = m.structure.View(m.width)
	case TabDetail:
		screen = m.detail.View(m.width)
	}

	// Error bar
	var errBar string
	if m.lastErr != "" {
		errBar = styles.StatusFailed.Render(fmt.Sprintf(" ⚠ %s", m.lastErr))
	}

	// Freeze indicator
	var freezeBar string
	if m.frozen {
		freezeBar = styles.StatusWarning.Render(" ⏸ FROZEN — press space to resume")
	}

	parts := []string{hdr, tabs}
	if freezeBar != "" {
		parts = append(parts, freezeBar)
	}
	parts = append(parts, screen)
	if errBar != "" {
		parts = append(parts, errBar)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// --- Internal helpers ---

func (m Model) renderTabBar() string {
	var parts []string
	for i, name := range tabNames {
		label := fmt.Sprintf(" %d:%s ", i+1, name)
		if Tab(i) == m.activeTab {
			parts = append(parts, styles.TabActive.Render(label))
		} else {
			parts = append(parts, styles.TabInactive.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m Model) updateActiveScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.activeTab {
	case TabLive:
		m.live, cmd = m.live.Update(msg)
	case TabFuture:
		m.future, cmd = m.future.Update(msg)
	case TabPast:
		m.past, cmd = m.past.Update(msg)
	case TabStructure:
		m.structure, cmd = m.structure.Update(msg)
	case TabDetail:
		m.detail, cmd = m.detail.Update(msg)
	}
	return m, cmd
}

func (m Model) forwardAndBatch(msg tea.Msg, extraCmds []tea.Cmd) (tea.Model, tea.Cmd) {
	updated, cmd := m.updateActiveScreen(msg)
	if cmd != nil {
		extraCmds = append(extraCmds, cmd)
	}
	if len(extraCmds) == 0 {
		return updated, nil
	}
	return updated, tea.Batch(extraCmds...)
}

func (m Model) renderHelp() string {
	help := `
  Ductile TUI — Keyboard Reference

  Navigation
    1-5         Switch tabs (Live/Future/Past/Structure/Detail)
    tab/S-tab   Cycle focus within screen
    ↑↓ / j k    Move selection
    enter       Open detail / drill down
    esc         Go back
    t           Open execution tree for selected job

  Actions
    r           Force refresh all data
    space       Freeze / unfreeze live updates
    /           Filter current view
    ?           Toggle this help
    q           Quit
`
	return lipgloss.NewStyle().Padding(2, 4).Render(help)
}

// --- Data fetch commands ---

func (m Model) fetchHealth() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		h, err := c.Health(context.Background())
		return msgs.HealthLoadedMsg{Data: h, Err: err}
	}
}

func (m Model) fetchJobs() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		jobs, total, err := c.ListJobs(context.Background(), "", "", "", 50)
		return msgs.JobsLoadedMsg{Data: jobs, Total: total, Err: err}
	}
}

func (m Model) fetchJobLogs() tea.Cmd {
	c := m.client
	dur := past.WindowDuration(m.past.Window)
	since := time.Now().Add(-dur)
	// Larger windows need more entries
	limit := 50
	if dur >= time.Hour {
		limit = 200
	}
	return func() tea.Msg {
		logs, total, err := c.ListJobLogs(context.Background(), "", "", &since, nil, limit)
		return msgs.JobLogsLoadedMsg{Data: logs, Total: total, Err: err}
	}
}

func (m Model) fetchSchedulerJobs() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		jobs, err := c.SchedulerJobs(context.Background())
		return msgs.SchedulerLoadedMsg{Data: jobs, Err: err}
	}
}

func (m Model) fetchPlugins() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		plugins, err := c.ListPlugins(context.Background())
		return msgs.PluginsLoadedMsg{Data: plugins, Err: err}
	}
}

func (m Model) fetchJobDetail(jobID string) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		d, err := c.GetJob(context.Background(), jobID)
		return msgs.JobDetailLoadedMsg{Data: d, Err: err}
	}
}

// --- SSE commands (ported from watch/client.go) ---

func (m Model) subscribeToEvents() tea.Cmd {
	apiURL := m.client.BaseURL()
	apiKey := m.client.APIKey()
	ch := m.sseEvents
	return func() tea.Msg {
		httpClient := &http.Client{}
		req, err := http.NewRequest("GET", apiURL+"/events", nil)
		if err != nil {
			return msgs.SSEDisconnectedMsg{}
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return msgs.SSEDisconnectedMsg{}
		}
		defer func() { _ = resp.Body.Close() }()

		scanner := bufio.NewScanner(resp.Body)
		var current struct {
			id   int64
			typ  string
			data string
		}

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if current.data != "" {
					ch <- types.Event{
						ID:   current.id,
						Type: current.typ,
						At:   time.Now(),
						Data: []byte(current.data),
					}
					current = struct {
						id   int64
						typ  string
						data string
					}{}
				}
				continue
			}

			if strings.HasPrefix(line, "id: ") {
				if id, err := strconv.ParseInt(line[4:], 10, 64); err == nil {
					current.id = id
				}
			} else if strings.HasPrefix(line, "event: ") {
				current.typ = line[7:]
			} else if strings.HasPrefix(line, "data: ") {
				current.data = line[6:]
			}
		}

		return msgs.SSEDisconnectedMsg{}
	}
}

func (m Model) receiveNextEvent() tea.Cmd {
	ch := m.sseEvents
	return func() tea.Msg {
		ev := <-ch
		return msgs.SSEEventMsg{Event: ev}
	}
}
