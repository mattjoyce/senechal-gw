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

// BuildReport renders a terminal-friendly lineage report for a job.
func BuildReport(ctx context.Context, db *sql.DB, statePath, jobID string) (string, error) {
	if strings.TrimSpace(jobID) == "" {
		return "", fmt.Errorf("job_id is required")
	}

	rootJob, err := lookupJob(ctx, db, jobID)
	if err != nil {
		return "", err
	}
	if !rootJob.contextID.Valid {
		return "", fmt.Errorf("job %q has no event_context_id", jobID)
	}

	contextStore := state.NewContextStore(db)
	lineage, err := contextStore.Lineage(ctx, rootJob.contextID.String)
	if err != nil {
		return "", fmt.Errorf("load context lineage: %w", err)
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Lineage Report\n")
	fmt.Fprintf(&out, "Job ID      : %s\n", rootJob.id)
	fmt.Fprintf(&out, "Plugin      : %s\n", rootJob.plugin)
	fmt.Fprintf(&out, "Command     : %s\n", rootJob.command)
	fmt.Fprintf(&out, "Status      : %s\n", rootJob.status)
	fmt.Fprintf(&out, "Context ID  : %s\n", rootJob.contextID.String)
	fmt.Fprintf(&out, "Hops        : %d\n", len(lineage))
	fmt.Fprintf(&out, "\n")

	workspaceBaseDir := workspaceBaseDirFromStatePath(statePath)
	for idx, node := range lineage {
		stepJob, _ := lookupFirstJobByContext(ctx, db, node.ID)
		fmt.Fprintf(&out, "[%d] %s :: %s\n", idx+1, renderUnset(node.PipelineName, "<root>"), renderUnset(node.StepID, "<entry>"))
		fmt.Fprintf(&out, "    context_id : %s\n", node.ID)
		if node.ParentID != nil {
			fmt.Fprintf(&out, "    parent_id  : %s\n", *node.ParentID)
		} else {
			fmt.Fprintf(&out, "    parent_id  : <none>\n")
		}

		if stepJob != nil {
			fmt.Fprintf(&out, "    job        : %s (%s:%s, %s)\n", stepJob.id, stepJob.plugin, stepJob.command, stepJob.status)
			workspaceDir := filepath.Join(workspaceBaseDir, stepJob.id)
			fmt.Fprintf(&out, "    workspace  : %s\n", workspaceDir)
			artifacts, err := listArtifacts(workspaceDir)
			if err != nil {
				fmt.Fprintf(&out, "    artifacts  : <error: %v>\n", err)
			} else if len(artifacts) == 0 {
				fmt.Fprintf(&out, "    artifacts  : <none>\n")
			} else {
				fmt.Fprintf(&out, "    artifacts  :\n")
				for _, artifact := range artifacts {
					fmt.Fprintf(&out, "      - %s\n", artifact)
				}
			}
		} else {
			fmt.Fprintf(&out, "    job        : <none>\n")
			fmt.Fprintf(&out, "    workspace  : <unknown>\n")
			fmt.Fprintf(&out, "    artifacts  : <unknown>\n")
		}

		fmt.Fprintf(&out, "    baggage    :\n")
		baggage := prettyJSON(node.AccumulatedJSON)
		for _, line := range strings.Split(strings.TrimSpace(baggage), "\n") {
			fmt.Fprintf(&out, "      %s\n", line)
		}
		fmt.Fprintf(&out, "\n")
	}

	return strings.TrimRight(out.String(), "\n") + "\n", nil
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
