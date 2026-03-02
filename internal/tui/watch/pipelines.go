package watch

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/events"
)

// PipelineState tracks a pipeline discovered from events.
type PipelineState struct {
	Name          string
	ActiveJobs    map[string]*JobState
	LastStatus    string // last completed job status
	LastRun       time.Time
	LastEventType string
	LastJobID     string
}

// JobState tracks an individual job execution.
type JobState struct {
	ID        string
	Plugin    string
	Command   string
	Pipeline  string
	StepID    string
	Status    string
	StartTime time.Time
	EndTime   time.Time
}

// PipelineCatalogEntry describes a configured pipeline or plugin.
type PipelineCatalogEntry struct {
	Name          string
	Kind          string
	Enabled       bool
	Trigger       string
	ExecutionMode string
	Timeout       time.Duration
	StepCount     int
	Uses          string
	ScheduleCount int
}

// PipelineListItem merges config catalog with runtime state.
type PipelineListItem struct {
	Entry PipelineCatalogEntry
	State *PipelineState
}

// updatePipelineState processes an event and updates pipeline/job tracking.
func updatePipelineState(pipelines map[string]*PipelineState, jobs map[string]*JobState, e events.Event) {
	data := make(map[string]any)
	_ = json.Unmarshal(e.Data, &data)

	jobID, _ := data["job_id"].(string)
	if jobID == "" {
		return
	}

	switch e.Type {
	case "job.started", "job.enqueued", "router.enqueued", "scheduler.scheduled":
		job, ok := jobs[jobID]
		if !ok {
			job = &JobState{ID: jobID}
			jobs[jobID] = job
		}

		if plugin, ok := data["plugin"].(string); ok {
			job.Plugin = plugin
		}
		if command, ok := data["command"].(string); ok {
			job.Command = command
		}
		if pipeline, ok := data["pipeline"].(string); ok {
			job.Pipeline = pipeline
		}
		if stepID, ok := data["step_id"].(string); ok {
			job.StepID = stepID
		}

		switch e.Type {
		case "job.started":
			job.Status = "running"
			job.StartTime = time.Now()
		default:
			if job.Status == "" {
				job.Status = "queued"
			}
		}

		// Track in pipeline
		pName := pipelineName(job)
		if pName != "" {
			p := getOrCreatePipeline(pipelines, pName)
			p.LastEventType = e.Type
			p.LastJobID = jobID
			if job.Status == "running" || job.Status == "queued" {
				p.ActiveJobs[jobID] = job
			}
		}

	case "job.completed", "job.failed", "job.timed_out":
		job, ok := jobs[jobID]
		if !ok {
			return
		}
		status, _ := data["status"].(string)
		job.Status = status
		job.EndTime = time.Now()

		pName := pipelineName(job)
		if p, ok := pipelines[pName]; ok {
			delete(p.ActiveJobs, jobID)
			p.LastStatus = status
			p.LastRun = time.Now()
			p.LastEventType = e.Type
			p.LastJobID = jobID
		}
	}
}

func pipelineName(job *JobState) string {
	if job.Pipeline != "" {
		return job.Pipeline
	}
	return job.Plugin // fallback to plugin name
}

func getOrCreatePipeline(pipelines map[string]*PipelineState, name string) *PipelineState {
	p, ok := pipelines[name]
	if !ok {
		p = &PipelineState{
			Name:       name,
			ActiveJobs: make(map[string]*JobState),
		}
		pipelines[name] = p
	}
	return p
}

// buildPipelineCatalog builds the configured pipeline catalog.
func buildPipelineCatalog(cfg *config.Config) []PipelineCatalogEntry {
	if cfg == nil {
		return nil
	}

	entries := make(map[string]PipelineCatalogEntry)

	for _, pipeline := range cfg.Pipelines {
		name := strings.TrimSpace(pipeline.Name)
		if name == "" {
			continue
		}
		entries[name] = PipelineCatalogEntry{
			Name:          name,
			Kind:          "pipeline",
			Enabled:       true,
			Trigger:       pipeline.On,
			ExecutionMode: string(pipeline.ExecutionMode),
			Timeout:       pipeline.Timeout,
			StepCount:     countPipelineSteps(pipeline.Steps),
		}
	}

	for name, plugin := range cfg.Plugins {
		if _, exists := entries[name]; exists {
			continue
		}
		entries[name] = PipelineCatalogEntry{
			Name:          name,
			Kind:          "plugin",
			Enabled:       plugin.Enabled,
			Uses:          plugin.Uses,
			ScheduleCount: len(plugin.NormalizedSchedules()),
		}
	}

	keys := make([]string, 0, len(entries))
	for name := range entries {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	catalog := make([]PipelineCatalogEntry, 0, len(keys))
	for _, name := range keys {
		catalog = append(catalog, entries[name])
	}
	return catalog
}

func countPipelineSteps(steps []config.StepEntry) int {
	count := 0
	for _, step := range steps {
		count++
		if len(step.Steps) > 0 {
			count += countPipelineSteps(step.Steps)
		}
		if len(step.Split) > 0 {
			count += countPipelineSteps(step.Split)
		}
	}
	return count
}

// buildPipelineList merges the catalog with active runtime state.
func buildPipelineList(catalog []PipelineCatalogEntry, pipelines map[string]*PipelineState) []PipelineListItem {
	items := make([]PipelineListItem, 0, len(catalog)+len(pipelines))
	seen := make(map[string]bool)

	for _, entry := range catalog {
		items = append(items, PipelineListItem{
			Entry: entry,
			State: pipelines[entry.Name],
		})
		seen[entry.Name] = true
	}

	for name, state := range pipelines {
		if seen[name] {
			continue
		}
		items = append(items, PipelineListItem{
			Entry: PipelineCatalogEntry{
				Name:    name,
				Kind:    "observed",
				Enabled: true,
			},
			State: state,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Entry.Name < items[j].Entry.Name
	})
	return items
}

func renderPipelines(items []PipelineListItem, selected int, theme Theme, width int, height int) string {
	innerWidth := width - 2

	if len(items) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left,
			theme.Dim.Render("  No pipeline activity yet..."),
		)
		return theme.Border.Width(innerWidth).Height(height).Render(content)
	}

	// Viewport logic
	visibleHeight := height - 2 // internal height minus borders
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	// Simple scroll logic: keep selected in view
	start := 0
	if selected >= visibleHeight {
		start = selected - visibleHeight + 1
	}

	var lines []string
	for i := start; i < len(items) && len(lines) < visibleHeight; i++ {
		line := renderPipelineRow(i+1, items[i], i == selected, theme, width-4)
		lines = append(lines, line)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	// Add scrollbar if needed
	if len(items) > visibleHeight {
		content = renderWithScrollbar(content, start, len(items), visibleHeight, theme)
	}

	return theme.Border.Width(innerWidth).Height(height).Render(content)
}

func renderPipelineRow(num int, item PipelineListItem, isSelected bool, theme Theme, width int) string {
	activeCount := 0
	if item.State != nil {
		activeCount = len(item.State.ActiveJobs)
	}

	statusStr := pipelineStatus(item, theme, activeCount)
	
	nameStyle := lipgloss.NewStyle()
	if isSelected {
		nameStyle = nameStyle.Bold(true).
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57"))
	} else if !item.Entry.Enabled {
		nameStyle = theme.StatusDead
	}

	maxNameLen := width - 20
	if maxNameLen < 10 {
		maxNameLen = 10
	}
	name := item.Entry.Name
	if len(name) > maxNameLen {
		name = name[:maxNameLen-3] + "..."
	}
	nameText := nameStyle.Render(fmt.Sprintf("%-*s", maxNameLen, name))

	line := fmt.Sprintf(" %d. %s %s", num, nameText, statusStr)

	// If selected, show one active job if any
	if isSelected && activeCount > 0 && item.State != nil {
		// Just show the first one to save space in the list view
		var firstJob *JobState
		for _, job := range item.State.ActiveJobs {
			firstJob = job
			break
		}
		if firstJob != nil {
			jobID := firstJob.ID
			if len(jobID) > 8 {
				jobID = jobID[:8]
			}
			line += fmt.Sprintf("\n    └─ %s %s", theme.Highlight.Render(jobID), firstJob.Plugin)
		}
	}

	return line
}

func renderWithScrollbar(content string, start, total, visible int, theme Theme) string {
	if total <= visible {
		return content
	}

	scrollbarHeight := visible
	thumbHeight := int(float64(visible) * float64(visible) / float64(total))
	if thumbHeight < 1 {
		thumbHeight = 1
	}

	thumbStart := int(float64(start) * float64(visible) / float64(total))
	if thumbStart+thumbHeight > scrollbarHeight {
		thumbStart = scrollbarHeight - thumbHeight
	}

	var sb strings.Builder
	lines := strings.Split(content, "\n")
	for i := 0; i < visible; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}

		char := "│"
		if i >= thumbStart && i < thumbStart+thumbHeight {
			char = theme.Highlight.Render("┃")
		} else {
			char = theme.Dim.Render("│")
		}

		// Ensure line is padded to width before adding scrollbar
		// This is tricky with lipgloss, so we just append.
		// The parent border will handle the overall width.
		sb.WriteString(line + " " + char + "\n")
	}

	return strings.TrimSuffix(sb.String(), "\n")
}

func activeJobCount(item PipelineListItem) int {
	if item.State == nil {
		return 0
	}
	return len(item.State.ActiveJobs)
}

func truncateJobID(jobID string) string {
	if len(jobID) > 8 {
		return jobID[:8]
	}
	return jobID
}

func pipelineStatus(item PipelineListItem, theme Theme, activeCount int) string {
	if !item.Entry.Enabled {
		return theme.StatusDead.Render("[off]")
	}
	if activeCount > 0 {
		return theme.StatusRunning.Render(fmt.Sprintf("[%da]", activeCount))
	}
	if item.State != nil && !item.State.LastRun.IsZero() {
		return statusIcon(item.State.LastStatus, theme)
	}
	return theme.Dim.Render("[id]")
}

func statusIcon(status string, theme Theme) string {
	switch status {
	case "succeeded":
		return theme.StatusOK.Render("✅")
	case "failed":
		return theme.StatusFailed.Render("❌")
	case "timed_out":
		return theme.StatusFailed.Render("⏱")
	default:
		return "•"
	}
}

func formatAgo(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
