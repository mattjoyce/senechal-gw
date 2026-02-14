package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/events"
)

// --- Styles ---

var (
	docStyle = lipgloss.NewStyle().Margin(1, 2)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#874BFD"))

	statusOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	statusRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))
	statusFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	statusQueued  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Padding(0, 1)
)

// --- Types ---

type JobNode struct {
	ID        string
	Plugin    string
	Command   string
	Pipeline  string
	StepID    string
	Status    string
	StartTime time.Time
	EndTime   time.Time
	Children  []*JobNode
	Parent    *JobNode
}

type Model struct {
	apiURL string
	apiKey string

	width  int
	height int

	systemNode *JobNode
	jobs       map[string]*JobNode
	eventLog   []events.Event
	hubEvents  chan events.Event

	health struct {
		Status        string
		UptimeSeconds int64
		QueueDepth    int
		PluginsLoaded int
	}

	jobTable table.Model
	viewport viewport.Model

	lastTick time.Time
	mu       sync.Mutex
}

type eventMsg events.Event
type healthMsg struct {
	Status        string `json:"status"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	QueueDepth    int    `json:"queue_depth"`
	PluginsLoaded int    `json:"plugins_loaded"`
}
type errMsg error

// --- Init ---

func NewMonitor(apiURL, apiKey string) *Model {
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "ST", Width: 2},
			{Title: "Tree (Plugin/Step)", Width: 35},
			{Title: "Command", Width: 10},
			{Title: "ID", Width: 10},
			{Title: "Duration", Width: 10},
		}),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return &Model{
		apiURL: apiURL,
		apiKey: apiKey,
		systemNode: &JobNode{
			ID:     "system",
			Plugin: "System",
			Status: "succeeded",
		},
		jobs:      make(map[string]*JobNode),
		eventLog:  make([]events.Event, 0),
		hubEvents: make(chan events.Event, 100),
		jobTable:  t,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.subscribeToEvents(),
		m.receiveNextEvent(), // Kickstart event loop
		m.pollHealth(),
		tea.EnterAltScreen,
	)
}

// --- Update ---

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.jobTable.SetWidth(m.width - 6)
		m.viewport.Width = m.width - 6
		m.viewport.Height = m.height / 3

	case eventMsg:
		m.handleEvent(events.Event(msg))
		m.updateTable()
		return m, m.receiveNextEvent()

	case healthMsg:
		m.health.Status = msg.Status
		m.health.UptimeSeconds = msg.UptimeSeconds
		m.health.QueueDepth = msg.QueueDepth
		m.health.PluginsLoaded = msg.PluginsLoaded
		return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return m.fetchHealth()
		})

	case errMsg:
		// Handle error
	}

	m.jobTable, cmd = m.jobTable.Update(msg)
	return m, cmd
}

func (m *Model) handleEvent(e events.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventLog = append([]events.Event{e}, m.eventLog...)
	if len(m.eventLog) > 50 {
		m.eventLog = m.eventLog[:50]
	}

	data := make(map[string]any)
	_ = json.Unmarshal(e.Data, &data)

	switch e.Type {
	case "job.started", "job.enqueued", "router.enqueued", "scheduler.scheduled":
		jobID, _ := data["job_id"].(string)
		if jobID == "" {
			return
		}
		node, ok := m.jobs[jobID]
		if !ok {
			node = &JobNode{ID: jobID}
			m.jobs[jobID] = node

			parentID, _ := data["parent_job_id"].(string)
			if parentID != "" {
				if parent, ok := m.jobs[parentID]; ok {
					node.Parent = parent
					parent.Children = append(parent.Children, node)
				} else {
					// Parent not in local view yet, attach to system for now
					node.Parent = m.systemNode
					m.systemNode.Children = append(m.systemNode.Children, node)
				}
			} else {
				node.Parent = m.systemNode
				m.systemNode.Children = append(m.systemNode.Children, node)
			}
		}

		if plugin, ok := data["plugin"].(string); ok {
			node.Plugin = plugin
		}
		if command, ok := data["command"].(string); ok {
			node.Command = command
		}
		if pipeline, ok := data["pipeline"].(string); ok {
			node.Pipeline = pipeline
		}
		if stepID, ok := data["step_id"].(string); ok {
			node.StepID = stepID
		}

		switch e.Type {
		case "job.started":
			node.Status = "running"
			node.StartTime = time.Now()
		case "job.enqueued", "router.enqueued", "scheduler.scheduled":
			if node.Status == "" {
				node.Status = "queued"
			}
		}

	case "job.completed", "job.failed", "job.timed_out":
		jobID, _ := data["job_id"].(string)
		if node, ok := m.jobs[jobID]; ok {
			node.Status, _ = data["status"].(string)
			node.EndTime = time.Now()
		}

	case "scheduler.tick":
		m.lastTick = time.Now()
	}
}

func (m *Model) updateTable() {
	var rows []table.Row

	// Skip the virtual "System" node itself in the rows, or show it as root.
	// Let's show it as root.
	rows = append(rows, m.nodeToRow(m.systemNode, "", true)...)

	m.jobTable.SetRows(rows)
}

func (m *Model) nodeToRow(node *JobNode, prefix string, isLast bool) []table.Row {
	if node == nil {
		return nil
	}

	var row table.Row
	statusSym := "〇"
	switch node.Status {
	case "queued":
		statusSym = statusQueued.Render("〇")
	case "running":
		statusSym = statusRunning.Render("◉")
	case "succeeded":
		statusSym = statusOK.Render("⦿")
	case "failed":
		statusSym = statusFailed.Render("∅")
	case "timed_out":
		statusSym = statusFailed.Render("⊖")
	case "dead":
		statusSym = statusFailed.Render("⊘")
	}

	duration := "-"
	if !node.StartTime.IsZero() {
		end := node.EndTime
		if end.IsZero() {
			end = time.Now()
		}
		duration = end.Sub(node.StartTime).Round(time.Millisecond).String()
	}

	// Build the tree line
	var treeLine string
	if node.ID == "system" {
		treeLine = node.Plugin
	} else {
		marker := "├─ "
		if isLast {
			marker = "└─ "
		}
		
		label := node.Plugin
		if node.StepID != "" {
			label = fmt.Sprintf("%s (%s)", node.Plugin, node.StepID)
		}
		treeLine = prefix + marker + label
	}

	displayID := node.ID
	if len(displayID) > 8 {
		displayID = displayID[:8]
	}

	row = table.Row{
		statusSym,
		treeLine,
		node.Command,
		displayID,
		duration,
	}

	rows := []table.Row{row}

	// Prepare prefix for children
	newPrefix := prefix
	if node.ID != "system" {
		if isLast {
			newPrefix += "   "
		} else {
			newPrefix += "│  "
		}
	}

	for i, child := range node.Children {
		lastChild := i == len(node.Children)-1
		rows = append(rows, m.nodeToRow(child, newPrefix, lastChild)...)
	}

	return rows
}

// --- View ---

func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	header := m.renderHeader()
	activeJobs := borderStyle.Width(m.width - 4).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Processes"),
			m.jobTable.View(),
		),
	)

	eventsView := borderStyle.Width(m.width - 4).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Event Stream"),
			m.renderEvents(),
		),
	)

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(" [q] Quit • [↑/↓] Scroll Jobs")

	return docStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			activeJobs,
			eventsView,
			help,
		),
	)
}

func (m Model) renderHeader() string {
	status := statusOK.Render("RUNNING")
	if m.health.Status != "ok" && m.health.Status != "" {
		status = statusFailed.Render("DEGRADED")
	}

	uptime := time.Duration(m.health.UptimeSeconds) * time.Second
	
	items := []string{
		fmt.Sprintf("Status: %s", status),
		fmt.Sprintf("Uptime: %s", uptime.String()),
		fmt.Sprintf("Queue: %d", m.health.QueueDepth),
		fmt.Sprintf("Plugins: %d", m.health.PluginsLoaded),
	}

	return borderStyle.Width(m.width - 4).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width((m.width-4)/4).Render(items[0]),
			lipgloss.NewStyle().Width((m.width-4)/4).Render(items[1]),
			lipgloss.NewStyle().Width((m.width-4)/4).Render(items[2]),
			lipgloss.NewStyle().Width((m.width-4)/4).Render(items[3]),
		),
	)
}

func (m Model) renderEvents() string {
	var lines []string
	for i, e := range m.eventLog {
		if i >= 10 {
			break
		}
		ts := e.At.Format("15:04:05")
		lines = append(lines, fmt.Sprintf("%s | %-15s | %s", ts, e.Type, string(e.Data)))
	}
	if len(lines) == 0 {
		return "  No events yet..."
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

// --- Commands ---

func (m Model) subscribeToEvents() tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{}
		req, _ := http.NewRequest("GET", m.apiURL+"/events", nil)
		req.Header.Set("Authorization", "Bearer "+m.apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return errMsg(err)
		}
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)

		// SSE events are multi-line: id, event, data
		var currentEvent struct {
			id   int64
			typ  string
			data string
		}

		for scanner.Scan() {
			line := scanner.Text()

			// Empty line marks end of event
			if line == "" {
				if currentEvent.data != "" {
					ev := events.Event{
						ID:   currentEvent.id,
						Type: currentEvent.typ,
						At:   time.Now(),
						Data: []byte(currentEvent.data),
					}
					m.hubEvents <- ev
					// Reset for next event
					currentEvent = struct {
						id   int64
						typ  string
						data string
					}{}
				}
				continue
			}

			// Parse SSE fields
			if strings.HasPrefix(line, "id: ") {
				if id, err := strconv.ParseInt(line[4:], 10, 64); err == nil {
					currentEvent.id = id
				}
			} else if strings.HasPrefix(line, "event: ") {
				currentEvent.typ = line[7:]
			} else if strings.HasPrefix(line, "data: ") {
				currentEvent.data = line[6:]
			}
			// Ignore other lines (e.g., ": keep-alive" comments)
		}
		return nil
	}
}

func (m Model) receiveNextEvent() tea.Cmd {
	return func() tea.Msg {
		return eventMsg(<-m.hubEvents)
	}
}

func (m Model) pollHealth() tea.Cmd {
	return func() tea.Msg {
		return m.fetchHealth()
	}
}

func (m Model) fetchHealth() tea.Msg {
	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest("GET", m.apiURL+"/healthz", nil)
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return errMsg(err)
	}
	defer resp.Body.Close()

	var h healthMsg
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return errMsg(err)
	}
	return h
}
