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

func renderPipelines(items []PipelineListItem, selected int, theme Theme, width int) string {
	innerWidth := width - 4

	if len(items) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left,
			theme.Title.Render("PIPELINES"),
			theme.Dim.Render("  No pipeline activity yet..."),
		)
		return theme.Border.Width(innerWidth).Render(content)
	}

	var lines []string
	for i, item := range items {
		line := renderPipelineRow(i+1, item, i == selected, theme)
		lines = append(lines, line)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		append([]string{theme.Title.Render("PIPELINES")}, lines...)...,
	)

	return theme.Border.Width(innerWidth).Render(content)
}

func renderPipelineRow(num int, item PipelineListItem, isSelected bool, theme Theme) string {
	activeCount := 0
	if item.State != nil {
		activeCount = len(item.State.ActiveJobs)
	}

	statusStr := pipelineStatus(item, theme, activeCount)
	lastRunStr := pipelineLastRun(item, theme)

	nameStyle := lipgloss.NewStyle()
	if isSelected {
		nameStyle = nameStyle.Bold(true).
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57"))
	} else if !item.Entry.Enabled {
		nameStyle = theme.StatusDead
	}

	nameText := nameStyle.Render(fmt.Sprintf("%-24s", item.Entry.Name))

	var line strings.Builder
	line.WriteString(fmt.Sprintf(" %d. %s  %s  %s",
		num,
		nameText,
		statusStr,
		lastRunStr,
	))

	if activeCount > 0 && item.State != nil {
		for _, job := range item.State.ActiveJobs {
			duration := "-"
			if !job.StartTime.IsZero() {
				duration = time.Since(job.StartTime).Round(time.Millisecond).String()
			}

			jobID := job.ID
			if len(jobID) > 8 {
				jobID = jobID[:8]
			}

			step := job.Plugin
			if job.Command != "" {
				step = job.Plugin + "/" + job.Command
			}

			jobLine := fmt.Sprintf("    └─ Job %s: %s %s",
				theme.Highlight.Render(jobID),
				step,
				theme.Dim.Render(duration),
			)
			line.WriteString("\n" + jobLine)
		}
	}

	return line.String()
}

func renderPipelineDetails(items []PipelineListItem, selected int, theme Theme, width int) string {
	innerWidth := width - 4

	if len(items) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left,
			theme.Title.Render("PIPELINE DETAILS"),
			theme.Dim.Render("  No pipeline configuration loaded."),
		)
		return theme.Border.Width(innerWidth).Render(content)
	}

	if selected < 0 || selected >= len(items) {
		selected = 0
	}

	item := items[selected]
	entry := item.Entry

	lines := []string{
		theme.Title.Render("PIPELINE DETAILS"),
		fmt.Sprintf(" Name: %s", theme.Highlight.Render(entry.Name)),
		fmt.Sprintf(" Type: %s", entry.Kind),
	}

	statusText := pipelineStatus(item, theme, activeJobCount(item))
	lines = append(lines, fmt.Sprintf(" Status: %s", statusText))

	if !entry.Enabled {
		lines = append(lines, fmt.Sprintf(" Enabled: %s", theme.StatusDead.Render("no")))
	} else {
		lines = append(lines, fmt.Sprintf(" Enabled: %s", theme.StatusOK.Render("yes")))
	}

	if item.State != nil && !item.State.LastRun.IsZero() {
		ago := time.Since(item.State.LastRun).Round(time.Second)
		lines = append(lines, fmt.Sprintf(" Last run: %s", formatAgo(ago)))
	}
	if item.State != nil && item.State.LastEventType != "" {
		lines = append(lines, fmt.Sprintf(" Last event: %s", item.State.LastEventType))
	}
	if item.State != nil && item.State.LastJobID != "" {
		lines = append(lines, fmt.Sprintf(" Last job: %s", truncateJobID(item.State.LastJobID)))
	}

	switch entry.Kind {
	case "pipeline":
		if entry.Trigger != "" {
			lines = append(lines, fmt.Sprintf(" Trigger: %s", entry.Trigger))
		}
		if entry.ExecutionMode != "" {
			lines = append(lines, fmt.Sprintf(" Mode: %s", entry.ExecutionMode))
		}
		if entry.Timeout > 0 {
			lines = append(lines, fmt.Sprintf(" Timeout: %s", entry.Timeout))
		}
		if entry.StepCount > 0 {
			lines = append(lines, fmt.Sprintf(" Steps: %d", entry.StepCount))
		}
	case "plugin":
		if entry.Uses != "" {
			lines = append(lines, fmt.Sprintf(" Uses: %s", entry.Uses))
		}
		if entry.ScheduleCount > 0 {
			lines = append(lines, fmt.Sprintf(" Schedules: %d", entry.ScheduleCount))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return theme.Border.Width(innerWidth).Render(content)
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
		return theme.StatusDead.Render("[disabled]")
	}
	if activeCount > 0 {
		return theme.StatusRunning.Render(fmt.Sprintf("[%d active]", activeCount))
	}
	return theme.Dim.Render("[idle]")
}

func pipelineLastRun(item PipelineListItem, theme Theme) string {
	if item.State == nil || item.State.LastRun.IsZero() {
		return ""
	}
	ago := time.Since(item.State.LastRun).Round(time.Second)
	icon := statusIcon(item.State.LastStatus, theme)
	return fmt.Sprintf("Last: %s %s", formatAgo(ago), icon)
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
		return ""
	}
}

func formatAgo(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}
