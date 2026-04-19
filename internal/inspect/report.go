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
	"time"

	"github.com/mattjoyce/ductile/internal/configsnapshot"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/state"
)

type jobInfo struct {
	id                       string
	plugin                   string
	command                  string
	status                   string
	attempt                  int
	contextID                sql.NullString
	enqueuedConfigSnapshotID sql.NullString
	startedConfigSnapshotID  sql.NullString
}

// Report is the structured JSON representation of a lineage report.
type Report struct {
	JobID       string             `json:"job_id"`
	Plugin      string             `json:"plugin"`
	Command     string             `json:"command"`
	Status      string             `json:"status"`
	Attempt     int                `json:"attempt"`
	ContextID   string             `json:"context_id"`
	Transitions []Transition       `json:"transitions"`
	Attempts    []Attempt          `json:"attempts"`
	Consistency LineageConsistency `json:"consistency"`
	Config      ConfigReport       `json:"config"`
	Hops        int                `json:"hops"`
	Steps       []Step             `json:"steps"`
}

// Transition is one observed job status transition.
type Transition struct {
	From   string `json:"from,omitempty"`
	To     string `json:"to"`
	At     string `json:"at"`
	Reason string `json:"reason,omitempty"`
}

// Attempt is one observed execution start.
type Attempt struct {
	Attempt int    `json:"attempt"`
	At      string `json:"at"`
}

// LineageConsistency compares cached queue state to append-only facts.
type LineageConsistency struct {
	LatestTransitionStatus string `json:"latest_transition_status,omitempty"`
	CachedStatusMatches    bool   `json:"cached_status_matches"`
	AttemptFactsMatch      bool   `json:"attempt_facts_match"`
	LegacyMissingData      bool   `json:"legacy_missing_data"`
}

// ConfigReport describes the runtime config values associated with a job.
type ConfigReport struct {
	Enqueued              *ConfigSnapshotSummary     `json:"enqueued,omitempty"`
	Started               *ConfigSnapshotSummary     `json:"started,omitempty"`
	CrossedReloadBoundary bool                       `json:"crossed_reload_boundary"`
	LegacyMissingData     bool                       `json:"legacy_missing_data"`
	MissingSnapshotRefs   []string                   `json:"missing_snapshot_refs,omitempty"`
	SecretUses            []configsnapshot.SecretUse `json:"secret_uses,omitempty"`
}

// ConfigSnapshotSummary is the inspect-safe subset of a config snapshot.
type ConfigSnapshotSummary struct {
	ID             string         `json:"id"`
	ConfigHash     string         `json:"config_hash"`
	SourceHash     string         `json:"source_hash,omitempty"`
	SourcePath     string         `json:"source_path,omitempty"`
	Source         string         `json:"source,omitempty"`
	Reason         string         `json:"reason"`
	LoadedAt       string         `json:"loaded_at"`
	DuctileVersion string         `json:"ductile_version,omitempty"`
	BinaryPath     string         `json:"binary_path,omitempty"`
	SnapshotFormat int            `json:"snapshot_format"`
	Semantics      map[string]any `json:"semantics,omitempty"`
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
	fmt.Fprintf(&out, "Attempt     : %d\n", report.Attempt)
	fmt.Fprintf(&out, "Context ID  : %s\n", report.ContextID)
	fmt.Fprintf(&out, "Hops        : %d\n", report.Hops)
	fmt.Fprintf(&out, "\n")

	fmt.Fprintf(&out, "Execution History\n")
	if len(report.Transitions) == 0 {
		fmt.Fprintf(&out, "  transitions : <none>\n")
	} else {
		fmt.Fprintf(&out, "  transitions :\n")
		for _, transition := range report.Transitions {
			from := transition.From
			if from == "" {
				from = "NULL"
			}
			if transition.Reason != "" {
				fmt.Fprintf(&out, "    - %s -> %s at %s (%s)\n", from, transition.To, transition.At, transition.Reason)
			} else {
				fmt.Fprintf(&out, "    - %s -> %s at %s\n", from, transition.To, transition.At)
			}
		}
	}
	if len(report.Attempts) == 0 {
		fmt.Fprintf(&out, "  attempts    : <none>\n")
	} else {
		fmt.Fprintf(&out, "  attempts    :\n")
		for _, attempt := range report.Attempts {
			fmt.Fprintf(&out, "    - attempt %d at %s\n", attempt.Attempt, attempt.At)
		}
	}
	fmt.Fprintf(&out, "  consistency : cached_status_matches=%t attempt_facts_match=%t legacy_missing_data=%t\n",
		report.Consistency.CachedStatusMatches,
		report.Consistency.AttemptFactsMatch,
		report.Consistency.LegacyMissingData,
	)
	fmt.Fprintf(&out, "\n")

	fmt.Fprintf(&out, "Config\n")
	renderConfigSnapshotLine(&out, "enqueued under", report.Config.Enqueued)
	renderConfigSnapshotLine(&out, "started under ", report.Config.Started)
	renderConfigSemanticsLine(&out, "enqueued semantics", report.Config.Enqueued)
	renderConfigSemanticsLine(&out, "started semantics ", report.Config.Started)
	fmt.Fprintf(&out, "  crossed reload boundary: %t\n", report.Config.CrossedReloadBoundary)
	fmt.Fprintf(&out, "  legacy missing data    : %t\n", report.Config.LegacyMissingData)
	if len(report.Config.MissingSnapshotRefs) > 0 {
		fmt.Fprintf(&out, "  missing snapshot refs  : %s\n", strings.Join(report.Config.MissingSnapshotRefs, ", "))
	}
	if len(report.Config.SecretUses) == 0 {
		fmt.Fprintf(&out, "  secrets                : <none>\n")
	} else {
		fmt.Fprintf(&out, "  secrets                :\n")
		for _, use := range report.Config.SecretUses {
			ref := use.Ref
			if ref == "" {
				ref = "<inline>"
			}
			fmt.Fprintf(&out, "    - %s used by %s present=%t\n", ref, use.Purpose, use.Present)
		}
	}
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
		for line := range strings.SplitSeq(strings.TrimSpace(baggage), "\n") {
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
		Attempt:   rootJob.attempt,
		ContextID: "",
		Hops:      0,
		Steps:     make([]Step, 0),
	}

	jobLineage, err := queue.New(db).GetJobLineage(ctx, rootJob.id)
	if err != nil {
		return nil, fmt.Errorf("load job lineage facts: %w", err)
	}
	report.Transitions = renderTransitions(jobLineage.Transitions)
	report.Attempts = renderAttempts(jobLineage.Attempts)
	report.Consistency = renderConsistency(jobLineage)
	report.Config = loadConfigReport(ctx, db, rootJob.enqueuedConfigSnapshotID, rootJob.startedConfigSnapshotID)

	if !rootJob.contextID.Valid {
		return report, nil
	}

	report.ContextID = rootJob.contextID.String

	contextStore := state.NewContextStore(db)
	contextLineage, err := contextStore.Lineage(ctx, rootJob.contextID.String)
	if err != nil {
		return nil, fmt.Errorf("load context lineage: %w", err)
	}
	report.Hops = len(contextLineage)
	report.Steps = make([]Step, 0, len(contextLineage))

	workspaceBaseDir := workspaceBaseDirFromStatePath(statePath)
	for idx, node := range contextLineage {
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
SELECT id, plugin, command, status, attempt, event_context_id, enqueued_config_snapshot_id, started_config_snapshot_id
FROM job_queue
WHERE id = ?;
`, jobID)
	if err := row.Scan(&info.id, &info.plugin, &info.command, &info.status, &info.attempt, &info.contextID,
		&info.enqueuedConfigSnapshotID, &info.startedConfigSnapshotID); err != nil {
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
SELECT id, plugin, command, status, attempt, event_context_id
FROM job_queue
WHERE event_context_id = ?
ORDER BY created_at ASC, rowid ASC
LIMIT 1;
`, contextID)
	if err := row.Scan(&info.id, &info.plugin, &info.command, &info.status, &info.attempt, &info.contextID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query job by context %q: %w", contextID, err)
	}
	return &info, nil
}

func renderTransitions(transitions []queue.JobTransition) []Transition {
	out := make([]Transition, 0, len(transitions))
	for _, transition := range transitions {
		rendered := Transition{
			To: string(transition.ToStatus),
			At: transition.CreatedAt.Format(time.RFC3339Nano),
		}
		if transition.FromStatus != nil {
			rendered.From = string(*transition.FromStatus)
		}
		if transition.Reason != nil {
			rendered.Reason = *transition.Reason
		}
		out = append(out, rendered)
	}
	return out
}

func renderAttempts(attempts []queue.JobAttempt) []Attempt {
	out := make([]Attempt, 0, len(attempts))
	for _, attempt := range attempts {
		out = append(out, Attempt{
			Attempt: attempt.Attempt,
			At:      attempt.CreatedAt.Format(time.RFC3339Nano),
		})
	}
	return out
}

func renderConsistency(lineage *queue.JobLineage) LineageConsistency {
	consistency := LineageConsistency{
		CachedStatusMatches: lineage.StatusMatchesLatest,
		AttemptFactsMatch:   lineage.AttemptFactsMatch,
		LegacyMissingData:   lineage.HasLegacyMissingData,
	}
	if lineage.LatestStatus != nil {
		consistency.LatestTransitionStatus = string(*lineage.LatestStatus)
	}
	return consistency
}

func loadConfigReport(ctx context.Context, db *sql.DB, enqueuedID, startedID sql.NullString) ConfigReport {
	report := ConfigReport{
		LegacyMissingData: !enqueuedID.Valid || !startedID.Valid,
	}
	if enqueuedID.Valid && startedID.Valid && enqueuedID.String != "" && startedID.String != "" {
		report.CrossedReloadBoundary = enqueuedID.String != startedID.String
	}

	if enqueuedID.Valid && strings.TrimSpace(enqueuedID.String) != "" {
		snapshot, err := configsnapshot.Get(ctx, db, enqueuedID.String)
		if err != nil {
			report.MissingSnapshotRefs = append(report.MissingSnapshotRefs, enqueuedID.String)
		} else {
			report.Enqueued = summarizeConfigSnapshot(snapshot)
			report.SecretUses = secretUsesFromSnapshot(snapshot)
		}
	}
	if startedID.Valid && strings.TrimSpace(startedID.String) != "" {
		snapshot, err := configsnapshot.Get(ctx, db, startedID.String)
		if err != nil {
			if !containsString(report.MissingSnapshotRefs, startedID.String) {
				report.MissingSnapshotRefs = append(report.MissingSnapshotRefs, startedID.String)
			}
		} else {
			report.Started = summarizeConfigSnapshot(snapshot)
			if report.Enqueued == nil || report.Enqueued.ID != snapshot.ID {
				report.SecretUses = mergeSecretUses(report.SecretUses, secretUsesFromSnapshot(snapshot))
			}
		}
	}
	return report
}

func summarizeConfigSnapshot(snapshot *configsnapshot.Snapshot) *ConfigSnapshotSummary {
	if snapshot == nil {
		return nil
	}
	summary := &ConfigSnapshotSummary{
		ID:             snapshot.ID,
		ConfigHash:     snapshot.ConfigHash,
		Reason:         snapshot.Reason,
		LoadedAt:       snapshot.LoadedAt.Format(time.RFC3339Nano),
		SnapshotFormat: snapshot.SnapshotFormat,
	}
	if snapshot.SourceHash != nil {
		summary.SourceHash = *snapshot.SourceHash
	}
	if snapshot.SourcePath != nil {
		summary.SourcePath = *snapshot.SourcePath
	}
	if snapshot.Source != nil {
		summary.Source = *snapshot.Source
	}
	if snapshot.DuctileVersion != nil {
		summary.DuctileVersion = *snapshot.DuctileVersion
	}
	if snapshot.BinaryPath != nil {
		summary.BinaryPath = *snapshot.BinaryPath
	}
	if len(snapshot.Semantics) > 0 {
		var semantics map[string]any
		if err := json.Unmarshal(snapshot.Semantics, &semantics); err == nil {
			summary.Semantics = semantics
		}
	}
	return summary
}

func secretUsesFromSnapshot(snapshot *configsnapshot.Snapshot) []configsnapshot.SecretUse {
	if snapshot == nil || len(snapshot.SecretFingerprints) == 0 {
		return nil
	}
	var uses []configsnapshot.SecretUse
	if err := json.Unmarshal(snapshot.SecretFingerprints, &uses); err != nil {
		return nil
	}
	return uses
}

func mergeSecretUses(existing, next []configsnapshot.SecretUse) []configsnapshot.SecretUse {
	seen := make(map[string]struct{}, len(existing)+len(next))
	out := make([]configsnapshot.SecretUse, 0, len(existing)+len(next))
	for _, use := range append(existing, next...) {
		key := use.Purpose + "\x00" + use.Ref + "\x00" + use.Fingerprint
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, use)
	}
	return out
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func renderConfigSnapshotLine(out *strings.Builder, label string, snapshot *ConfigSnapshotSummary) {
	if snapshot == nil {
		fmt.Fprintf(out, "  %s: <missing>\n", label)
		return
	}
	fmt.Fprintf(out, "  %s: %s %s %s\n", label, snapshot.ID, snapshot.Reason, snapshot.LoadedAt)
}

func renderConfigSemanticsLine(out *strings.Builder, label string, snapshot *ConfigSnapshotSummary) {
	if snapshot == nil || len(snapshot.Semantics) == 0 {
		return
	}
	keys := make([]string, 0, len(snapshot.Semantics))
	for key := range snapshot.Semantics {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, snapshot.Semantics[key]))
	}
	fmt.Fprintf(out, "  %s: %s\n", label, strings.Join(parts, ", "))
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
