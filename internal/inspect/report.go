package inspect

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mattjoyce/senechal-gw/internal/state"
)

type jobInfo struct {
	id        string
	plugin    string
	command   string
	status    string
	contextID sql.NullString
}

// Report is the structured JSON representation of a lineage report.
type Report struct {
	JobID     string `json:"job_id"`
	Plugin    string `json:"plugin"`
	Command   string `json:"command"`
	Status    string `json:"status"`
	ContextID string `json:"context_id"`
	Hops      int    `json:"hops"`
	Steps     []Step `json:"steps"`
}

// Step is one entry in the execution lineage.
type Step struct {
	Hop           int             `json:"hop"`
	Pipeline      string          `json:"pipeline"`
	StepID        string          `json:"step_id"`
	ContextID     string          `json:"context_id"`
	ParentID      string          `json:"parent_id,omitempty"`
	JobID         string          `json:"job_id,omitempty"`
	Plugin        string          `json:"plugin,omitempty"`
	Command       string          `json:"command,omitempty"`
	Status        string          `json:"status,omitempty"`
	WorkspacePath string          `json:"workspace_path,omitempty"`
	Artifacts     []string        `json:"artifacts,omitempty"`
	Baggage       json.RawMessage `json:"baggage"`
}

// BuildReport renders a terminal-friendly lineage report for a job.
func BuildReport(ctx context.Context, db *sql.DB, statePath, jobID string) (string, error) {
	report, err := gatherReportData(ctx, db, statePath, jobID)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Lineage Report\n")
	fmt.Fprintf(&out, "Job ID      : %s\n", report.JobID)
	fmt.Fprintf(&out, "Plugin      : %s\n", report.Plugin)
	fmt.Fprintf(&out, "Command     : %s\n", report.Command)
	fmt.Fprintf(&out, "Status      : %s\n", report.Status)
	fmt.Fprintf(&out, "Context ID  : %s\n", report.ContextID)
	fmt.Fprintf(&out, "Hops        : %d\n", report.Hops)
	fmt.Fprintf(&out, "\n")

	for _, step := range report.Steps {
		fmt.Fprintf(&out, "[%d] %s :: %s\n", step.Hop, step.Pipeline, step.StepID)
		fmt.Fprintf(&out, "    context_id : %s\n", step.ContextID)
		if step.ParentID != "" {
			fmt.Fprintf(&out, "    parent_id  : %s\n", step.ParentID)
		} else {
			fmt.Fprintf(&out, "    parent_id  : <none>\n")
		}

		if step.JobID != "" {
			fmt.Fprintf(&out, "    job        : %s (%s:%s, %s)\n", step.JobID, step.Plugin, step.Command, step.Status)
			fmt.Fprintf(&out, "    workspace  : %s\n", step.WorkspacePath)
			if len(step.Artifacts) == 0 {
				fmt.Fprintf(&out, "    artifacts  : <none>\n")
			} else {
				fmt.Fprintf(&out, "    artifacts  :\n")
				for _, artifact := range step.Artifacts {
					fmt.Fprintf(&out, "      - %s\n", artifact)
				}
			}
		} else {
			fmt.Fprintf(&out, "    job        : <none>\n")
			fmt.Fprintf(&out, "    workspace  : <unknown>\n")
			fmt.Fprintf(&out, "    artifacts  : <unknown>\n")
		}

		fmt.Fprintf(&out, "    baggage    :\n")
		baggage := prettyJSON(step.Baggage)
		for _, line := range strings.Split(strings.TrimSpace(baggage), "\n") {
			fmt.Fprintf(&out, "      %s\n", line)
		}
		fmt.Fprintf(&out, "\n")
	}

	return strings.TrimRight(out.String(), "\n") + "\n", nil
}

// BuildJSONReport returns the machine-readable JSON lineage report.
func BuildJSONReport(ctx context.Context, db *sql.DB, statePath, jobID string) (string, error) {
	report, err := gatherReportData(ctx, db, statePath, jobID)
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal json report: %w", err)
	}
	return string(data), nil
}

func gatherReportData(ctx context.Context, db *sql.DB, statePath, jobID string) (*Report, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	rootJob, err := lookupJob(ctx, db, jobID)
	if err != nil {
		return nil, err
	}

	report := &Report{
		JobID:     rootJob.id,
		Plugin:    rootJob.plugin,
		Command:   rootJob.command,
		Status:    rootJob.status,
		ContextID: "",
		Hops:      0,
		Steps:     make([]Step, 0),
	}
	if !rootJob.contextID.Valid {
		return report, nil
	}

	report.ContextID = rootJob.contextID.String

	contextStore := state.NewContextStore(db)
	lineage, err := contextStore.Lineage(ctx, rootJob.contextID.String)
	if err != nil {
		return nil, fmt.Errorf("load context lineage: %w", err)
	}
	report.Hops = len(lineage)
	report.Steps = make([]Step, 0, len(lineage))

	workspaceBaseDir := workspaceBaseDirFromStatePath(statePath)
	for idx, node := range lineage {
		stepJob, _ := lookupFirstJobByContext(ctx, db, node.ID)
		step := Step{
			Hop:       idx + 1,
			Pipeline:  renderUnset(node.PipelineName, "<root>"),
			StepID:    renderUnset(node.StepID, "<entry>"),
			ContextID: node.ID,
			Baggage:   node.AccumulatedJSON,
		}
		if node.ParentID != nil {
			step.ParentID = *node.ParentID
		}

		if stepJob != nil {
			step.JobID = stepJob.id
			step.Plugin = stepJob.plugin
			step.Command = stepJob.command
			step.Status = stepJob.status
			step.WorkspacePath = filepath.Join(workspaceBaseDir, stepJob.id)
			artifacts, _ := listArtifacts(step.WorkspacePath)
			step.Artifacts = artifacts
		}

		report.Steps = append(report.Steps, step)
	}

	return report, nil
}

func lookupJob(ctx context.Context, db *sql.DB, jobID string) (*jobInfo, error) {
	var info jobInfo
	row := db.QueryRowContext(ctx, `
SELECT id, plugin, command, status, event_context_id
FROM job_queue
WHERE id = ?;
`, jobID)
	if err := row.Scan(&info.id, &info.plugin, &info.command, &info.status, &info.contextID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("job %q not found", jobID)
		}
		return nil, fmt.Errorf("query job %q: %w", jobID, err)
	}
	return &info, nil
}

func lookupFirstJobByContext(ctx context.Context, db *sql.DB, contextID string) (*jobInfo, error) {
	var info jobInfo
	row := db.QueryRowContext(ctx, `
SELECT id, plugin, command, status, event_context_id
FROM job_queue
WHERE event_context_id = ?
ORDER BY created_at ASC, rowid ASC
LIMIT 1;
`, contextID)
	if err := row.Scan(&info.id, &info.plugin, &info.command, &info.status, &info.contextID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query job by context %q: %w", contextID, err)
	}
	return &info, nil
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}

func workspaceBaseDirFromStatePath(statePath string) string {
	return filepath.Join(filepath.Dir(statePath), "workspaces")
}

func listArtifacts(workspaceDir string) ([]string, error) {
	if _, err := os.Stat(workspaceDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	artifacts := make([]string, 0)
	err := filepath.WalkDir(workspaceDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == workspaceDir || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(workspaceDir, path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(artifacts)
	return artifacts, nil
}

func renderUnset(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
