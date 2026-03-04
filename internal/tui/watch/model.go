package watch

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/events"
)

const defaultHeartbeatInterval = time.Minute

// Column identifies the left or right side of the main layout.
type Column int

const (
	ColLeft Column = iota
	ColRight
)

// TabLeft identifies the active tab in the left column.
type TabLeft int

const (
	TabPipelines TabLeft = iota
	TabScheduler
)

// TabRight identifies the active tab in the right column.
type TabRight int

const (
	TabEvents TabRight = iota
	TabInspector
)

// Model is the main BubbleTea model for the watch TUI.
type Model struct {
	apiURL string
	apiKey string

	width  int
	height int

	// State
	health          HealthState
	pipelineCatalog []PipelineCatalogEntry
	pipelines       map[string]*PipelineState
	schedules       map[string]*ScheduleState
	jobs            map[string]*JobState
	eventLog        []events.Event

	// Live indicators
	heartbeat Heartbeat
	spinner   Spinner

	// UI state
	theme            Theme
	focusedCol       Column
	activeTabL       TabLeft
	activeTabR       TabRight
	selectedPipeline int
	pipelineScroll   int
	scheduleScroll   int
	eventScroll      int

	// Communication
	hubEvents chan events.Event

	// Error display
	lastError string
}

// New creates a new watch TUI model.
func New(apiURL, apiKey string, cfg *config.Config) *Model {
	return &Model{
		apiURL:           apiURL,
		apiKey:           apiKey,
		pipelineCatalog:  buildPipelineCatalog(cfg),
		pipelines:        make(map[string]*PipelineState),
		schedules:        make(map[string]*ScheduleState),
		jobs:             make(map[string]*JobState),
		eventLog:         make([]events.Event, 0),
		hubEvents:        make(chan events.Event, 100),
		heartbeat:        NewHeartbeat(),
		spinner:          NewSpinner(),
		theme:            NewDefaultTheme(),
		focusedCol:       ColLeft,
		activeTabL:       TabPipelines,
		activeTabR:       TabEvents,
		selectedPipeline: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		subscribeToEvents(m.apiURL, m.apiKey, m.hubEvents),
		receiveNextEvent(m.hubEvents),
		func() tea.Msg { return fetchHealth(m.apiURL, m.apiKey) },
		func() tea.Msg { return fetchSchedulerSnapshot(m.apiURL, m.apiKey) },
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
		tea.EnterAltScreen,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.focusedCol == ColLeft {
				m.focusedCol = ColRight
			} else {
				m.focusedCol = ColLeft
			}
		case "1":
			m.activeTabL = TabPipelines
			m.focusedCol = ColLeft
		case "2":
			m.activeTabL = TabScheduler
			m.focusedCol = ColLeft
		case "3":
			m.activeTabR = TabEvents
			m.focusedCol = ColRight
		case "4":
			m.activeTabR = TabInspector
			m.focusedCol = ColRight

		case "up", "k":
			m.handleUp()
		case "down", "j":
			m.handleDown()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		m.spinner.Decay()
		return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })

	case eventMsg:
		e := events.Event(msg)

		if e.Type == "scheduler.tick" {
			m.heartbeat.OnTick()
			m.health.Connected = true
			m.lastError = ""
			return m, receiveNextEvent(m.hubEvents)
		}

		// Update event log (newest first)
		m.eventLog = append([]events.Event{e}, m.eventLog...)
		if len(m.eventLog) > 100 {
			m.eventLog = m.eventLog[:100]
		}

		// Update spinner
		m.spinner.OnEvent()

		// Update pipeline/job state
		updatePipelineState(m.pipelines, m.jobs, e)
		updateScheduleState(m.schedules, e)

		// Mark as connected
		m.health.Connected = true
		m.lastError = ""

		return m, receiveNextEvent(m.hubEvents)

	case healthMsg:
		m.health.Status = msg.Status
		m.health.UptimeSeconds = msg.UptimeSeconds
		m.health.QueueDepth = msg.QueueDepth
		m.health.PluginsLoaded = msg.PluginsLoaded
		m.health.ConfigPath = msg.ConfigPath
		m.health.BinaryPath = msg.BinaryPath
		m.health.Version = msg.Version
		m.health.Connected = true
		m.health.LastCheck = time.Now()
		m.lastError = ""

		return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return fetchHealth(m.apiURL, m.apiKey)
		})

	case schedulerSnapshotMsg:
		applyScheduleSnapshot(m.schedules, msg)
		return m, nil

	case sseDisconnectedMsg:
		m.health.Connected = false
		m.lastError = "SSE disconnected, reconnecting..."
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return reconnectMsg{}
		})

	case reconnectMsg:
		return m, subscribeToEvents(m.apiURL, m.apiKey, m.hubEvents)

	case errMsg:
		m.lastError = msg.Error()
		return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return fetchHealth(m.apiURL, m.apiKey)
		})
	}

	return m, nil
}

func (m *Model) handleUp() {
	if m.focusedCol == ColLeft {
		if m.activeTabL == TabPipelines {
			if m.selectedPipeline > 0 {
				m.selectedPipeline--
			}
		} else if m.activeTabL == TabScheduler {
			if m.scheduleScroll > 0 {
				m.scheduleScroll--
			}
		}
	} else {
		if m.activeTabR == TabEvents {
			if m.eventScroll > 0 {
				m.eventScroll--
			}
		}
	}
}

func (m *Model) handleDown() {
	if m.focusedCol == ColLeft {
		if m.activeTabL == TabPipelines {
			items := buildPipelineList(m.pipelineCatalog, m.pipelines)
			if m.selectedPipeline < len(items)-1 {
				m.selectedPipeline++
			}
		} else if m.activeTabL == TabScheduler {
			m.scheduleScroll++
		}
	} else {
		if m.activeTabR == TabEvents {
			m.eventScroll++
		}
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "Initializing Overwatch..."
	}

	header := renderHeader(m.health, m.heartbeat, m.spinner, m.theme, m.width, defaultHeartbeatInterval, m.apiURL)

	// Calculate panel sizes
	mainHeight := m.height - lipgloss.Height(header) - 3 // footer and margin
	colWidth := (m.width - 4) / 2

	// Render Left Column
	var leftBody string
	var leftTabs string
	switch m.activeTabL {
	case TabPipelines:
		leftTabs = m.renderTabs([]string{"[1] Pipelines", "[2] Scheduler"}, 0, m.focusedCol == ColLeft)
		items := buildPipelineList(m.pipelineCatalog, m.pipelines)
		leftBody = renderPipelines(items, m.selectedPipeline, m.theme, colWidth, mainHeight-2)
	case TabScheduler:
		leftTabs = m.renderTabs([]string{"[1] Pipelines", "[2] Scheduler"}, 1, m.focusedCol == ColLeft)
		leftBody = renderSchedules(m.schedules, m.theme, colWidth, mainHeight-2, m.scheduleScroll)
	}

	// Render Right Column
	var rightBody string
	var rightTabs string
	switch m.activeTabR {
	case TabEvents:
		rightTabs = m.renderTabs([]string{"[3] Events", "[4] Inspector"}, 0, m.focusedCol == ColRight)
		rightBody = renderEventStream(m.eventLog, m.theme, colWidth, mainHeight-2, m.eventScroll)
	case TabInspector:
		rightTabs = m.renderTabs([]string{"[3] Events", "[4] Inspector"}, 1, m.focusedCol == ColRight)
		rightBody = m.renderInspector(colWidth, mainHeight-2)
	}

	leftPanel := lipgloss.JoinVertical(lipgloss.Left, leftTabs, leftBody)
	rightPanel := lipgloss.JoinVertical(lipgloss.Left, rightTabs, rightBody)

	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// Error bar
	var errBar string
	if m.lastError != "" {
		errBar = m.theme.StatusFailed.Render(fmt.Sprintf(" ⚠ %s", m.lastError))
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(" [q] Quit • [Tab] Switch Column • [1-4] Tabs • [↑/↓] Scroll")

	parts := []string{header, mainContent}
	if errBar != "" {
		parts = append(parts, errBar)
	}
	parts = append(parts, help)

	return lipgloss.NewStyle().Margin(1, 1).Render(
		lipgloss.JoinVertical(lipgloss.Left, parts...),
	)
}

func (m Model) renderTabs(labels []string, activeIndex int, isFocused bool) string {
	var tabs []string
	for i, label := range labels {
		style := m.theme.TabInactive
		if i == activeIndex {
			style = m.theme.TabActive
			if !isFocused {
				// Dim the active tab if its column isn't focused
				style = style.Copy().Background(lipgloss.Color("240"))
			}
		}
		tabs = append(tabs, style.Render(label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m Model) renderInspector(width, height int) string {
	// Placeholder for now
	content := m.theme.Dim.Render("  Job Inspector - select a job to see details")
	return m.theme.Border.Width(width - 2).Height(height).Render(content)
}
