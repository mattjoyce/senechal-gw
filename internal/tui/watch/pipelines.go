package watch

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattjoyce/ductile/internal/events"
)

// PipelineState tracks a pipeline discovered from events.
type PipelineState struct {
	Name       string
	ActiveJobs map[string]*JobState
	LastStatus string // last completed job status
	LastRun    time.Time
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

// sortedPipelineNames returns pipeline names in stable sorted order.
func sortedPipelineNames(pipelines map[string]*PipelineState) []string {
	names := make([]string, 0, len(pipelines))
	for name := range pipelines {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func renderPipelines(pipelines map[string]*PipelineState, selected int, theme Theme, width int) string {
	innerWidth := width - 4

	if len(pipelines) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left,
			theme.Title.Render("PIPELINES"),
			theme.Dim.Render("  No pipeline activity yet..."),
		)
		return theme.Border.Width(innerWidth).Render(content)
	}

	names := sortedPipelineNames(pipelines)

	var lines []string
	for i, name := range names {
		p := pipelines[name]
		line := renderPipelineRow(i+1, p, i == selected, theme)
		lines = append(lines, line)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		append([]string{theme.Title.Render("PIPELINES")}, lines...)...,
	)

	return theme.Border.Width(innerWidth).Render(content)
}

func renderPipelineRow(num int, p *PipelineState, isSelected bool, theme Theme) string {
	activeCount := len(p.ActiveJobs)

	// Status indicator
	var statusStr string
	if activeCount > 0 {
		statusStr = theme.StatusRunning.Render(fmt.Sprintf("[%d active]", activeCount))
	} else {
		statusStr = theme.Dim.Render("[idle]")
	}

	// Last run info
	var lastRunStr string
	if !p.LastRun.IsZero() {
		ago := time.Since(p.LastRun).Round(time.Second)
		icon := statusIcon(p.LastStatus, theme)
		lastRunStr = fmt.Sprintf("Last: %s %s", formatAgo(ago), icon)
	}

	// Build line
	nameStyle := lipgloss.NewStyle()
	if isSelected {
		nameStyle = nameStyle.Bold(true).
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57"))
	}

	var line strings.Builder
	line.WriteString(fmt.Sprintf(" %d. %s  %s  %s",
		num,
		nameStyle.Render(fmt.Sprintf("%-24s", p.Name)),
		statusStr,
		lastRunStr,
	))

	// Show active jobs underneath
	if activeCount > 0 {
		for _, job := range p.ActiveJobs {
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
