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
	selectedPipeline int

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
		selectedPipeline: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		subscribeToEvents(m.apiURL, m.apiKey, m.hubEvents),
		receiveNextEvent(m.hubEvents),
		func() tea.Msg { return fetchHealth(m.apiURL, m.apiKey) },
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
		case "up", "k":
			if m.selectedPipeline > 0 {
				m.selectedPipeline--
			}
		case "down", "j":
			items := buildPipelineList(m.pipelineCatalog, m.pipelines)
			if m.selectedPipeline < len(items)-1 {
				m.selectedPipeline++
			}
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
		if len(m.eventLog) > 50 {
			m.eventLog = m.eventLog[:50]
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

	case sseDisconnectedMsg:
		m.health.Connected = false
		m.lastError = "SSE disconnected, reconnecting..."
		// Reconnect after a short delay; the existing receiveNextEvent
		// goroutine is still waiting on the channel and will pick up
		// events from the new subscription.
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return reconnectMsg{}
		})

	case reconnectMsg:
		return m, subscribeToEvents(m.apiURL, m.apiKey, m.hubEvents)

	case errMsg:
		m.lastError = msg.Error()
		// Retry health in 5s
		return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return fetchHealth(m.apiURL, m.apiKey)
		})
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "Initializing Overwatch..."
	}

	items := buildPipelineList(m.pipelineCatalog, m.pipelines)
	selected := m.selectedPipeline
	if selected >= len(items) && len(items) > 0 {
		selected = len(items) - 1
	}

	header := renderHeader(m.health, m.heartbeat, m.spinner, m.theme, m.width, defaultHeartbeatInterval, m.apiURL)
	pipelines := renderPipelines(items, selected, m.theme, m.width)
	details := renderPipelineDetails(items, selected, m.theme, m.width)
	schedules := renderSchedules(m.schedules, m.theme, m.width)

	// Error bar
	var errBar string
	if m.lastError != "" {
		errBar = m.theme.StatusFailed.Render(fmt.Sprintf(" ⚠ %s", m.lastError))
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(" [q] Quit • [↑/↓] Navigate Pipelines")

	availableHeight := m.height - 2
	usedHeight := lipgloss.Height(header) + lipgloss.Height(pipelines) + lipgloss.Height(details) + lipgloss.Height(schedules) + lipgloss.Height(help)
	if errBar != "" {
		usedHeight += lipgloss.Height(errBar)
	}

	eventHeight := availableHeight - usedHeight
	maxEvents := eventHeight - 3
	if maxEvents < 1 {
		maxEvents = 1
	}
	eventStream := renderEventStream(m.eventLog, m.theme, m.width, maxEvents)

	parts := []string{header, pipelines, details, schedules, eventStream}
	if errBar != "" {
		parts = append(parts, errBar)
	}
	parts = append(parts, help)

	return lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, parts...),
	)
}
