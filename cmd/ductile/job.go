package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/inspect"
	"github.com/mattjoyce/ductile/internal/queue"
	"github.com/mattjoyce/ductile/internal/storage"
)

func runJobNoun(args []string) int {
	if len(args) < 1 {
		printJobNounHelp(os.Stderr)
		return 1
	}

	if isHelpToken(args[0]) {
		printJobNounHelp(os.Stdout)
		return 0
	}

	action := args[0]
	actionArgs := args[1:]

	switch action {
	case "inspect":
		if hasHelpFlag(actionArgs) {
			printJobInspectHelp()
			return 0
		}
		return runInspect(actionArgs)
	case "logs":
		if hasHelpFlag(actionArgs) {
			printJobLogsHelp()
			return 0
		}
		return runJobLogs(actionArgs)
	case "help":
		printJobNounHelp(os.Stdout)
		return 0
	default:
		// #nosec G705 -- stderr output is plain text, not HTML.
		fmt.Fprintf(os.Stderr, "Unknown job action: %s\n", action)
		return 1
	}
}

func printJobNounHelp(w *os.File) {
	_, _ = fmt.Fprintln(w, "Usage: ductile job <action>")
	_, _ = fmt.Fprintln(w, "Actions: inspect, logs")
}

func printJobInspectHelp() {
	fmt.Println("Usage: ductile job inspect <job_id> [--config PATH] [--json]")
	fmt.Println("Inspect job lineage, baggage, and execution details.")
}

func printJobLogsHelp() {
	fmt.Println("Usage: ductile job logs [--config PATH] [--json] [--plugin NAME] [--command CMD] [--status STATUS] [--submitted-by NAME] [--from TIME] [--to TIME] [--query TEXT] [--limit N] [--include-result]")
	fmt.Println("Query stored job logs for audit and troubleshooting.")
	fmt.Println("Time values must be RFC3339 (e.g. 2025-01-02T15:04:05Z).")
}

func runInspect(args []string) int {
	// Custom flag parsing because we want to support flags intermixed with the job ID
	// like 'ductile job inspect <id> --json' or 'ductile job inspect --json <id>'
	var configPath string
	var jsonOut bool

	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to configuration")
	fs.BoolVar(&jsonOut, "json", false, "Output report in JSON")

	var jobID string
	remainingArgs := args
	for len(remainingArgs) > 0 {
		if err := fs.Parse(remainingArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
			return 1
		}
		if fs.NArg() > 0 {
			if jobID == "" {
				jobID = fs.Arg(0)
			}
			remainingArgs = fs.Args()[1:]
		} else {
			remainingArgs = nil
		}
	}

	if jobID == "" {
		fmt.Fprintf(os.Stderr, "Usage: ductile job inspect <job_id> [--config PATH] [--json]\n")
		return 1
	}

	if configPath == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to discover config: %v\n", err)
			return 1
		}
		configPath = discovered
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return 1
	}

	db, err := storage.OpenSQLite(context.Background(), cfg.State.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	var report string
	if jsonOut {
		report, err = inspect.BuildJSONReport(context.Background(), db, cfg.State.Path, jobID)
	} else {
		report, err = inspect.BuildReport(context.Background(), db, cfg.State.Path, jobID)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Inspect failed: %v\n", err)
		return 1
	}

	fmt.Print(report)
	return 0
}

func runJobLogs(args []string) int {
	fs := flag.NewFlagSet("job logs", flag.ContinueOnError)
	configPath := fs.String("config", "", "Path to configuration")
	jsonOut := fs.Bool("json", false, "Output JSON")
	plugin := fs.String("plugin", "", "Filter by plugin")
	command := fs.String("command", "", "Filter by command")
	statusRaw := fs.String("status", "", "Filter by status")
	submittedBy := fs.String("submitted-by", "", "Filter by submitted_by")
	fromRaw := fs.String("from", "", "Filter by completed_at >= from (RFC3339)")
	toRaw := fs.String("to", "", "Filter by completed_at <= to (RFC3339)")
	query := fs.String("query", "", "Search last_error/stderr/result")
	limit := fs.Int("limit", 50, "Max rows (<=200)")
	includeResult := fs.Bool("include-result", false, "Include full result payloads")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if fs.NArg() > 0 {
		printJobLogsHelp()
		return 1
	}
	if *limit <= 0 || *limit > 200 {
		fmt.Fprintln(os.Stderr, "limit must be between 1 and 200")
		return 1
	}

	if *configPath == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to discover config: %v\n", err)
			return 1
		}
		*configPath = discovered
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return 1
	}

	db, err := storage.OpenSQLite(context.Background(), cfg.State.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	filter := queue.JobLogFilter{
		Plugin:        strings.TrimSpace(*plugin),
		Command:       strings.TrimSpace(*command),
		SubmittedBy:   strings.TrimSpace(*submittedBy),
		Query:         strings.TrimSpace(*query),
		Limit:         *limit,
		IncludeResult: *includeResult,
	}

	if strings.TrimSpace(*statusRaw) != "" {
		status, ok := parseJobStatusFlag(*statusRaw)
		if !ok {
			fmt.Fprintln(os.Stderr, "invalid status filter")
			return 1
		}
		filter.Status = &status
	}

	if strings.TrimSpace(*fromRaw) != "" {
		parsed, err := parseTimeFlag(*fromRaw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid from timestamp")
			return 1
		}
		filter.Since = &parsed
	}

	if strings.TrimSpace(*toRaw) != "" {
		parsed, err := parseTimeFlag(*toRaw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid to timestamp")
			return 1
		}
		filter.Until = &parsed
	}

	q := queue.New(db)
	logs, total, err := q.ListJobLogs(context.Background(), filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query job logs: %v\n", err)
		return 1
	}

	if *jsonOut {
		out := struct {
			Total int                  `json:"total"`
			Logs  []*queue.JobLogEntry `json:"logs"`
		}{
			Total: total,
			Logs:  logs,
		}
		payload, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
			return 1
		}
		fmt.Println(string(payload))
		return 0
	}

	fmt.Printf("Job Logs (total=%d)\n", total)
	for _, entry := range logs {
		fmt.Printf("- %s %s %s:%s job=%s attempt=%d submitted_by=%s\n", entry.CompletedAt.Format(time.RFC3339), entry.Status, entry.Plugin, entry.Command, entry.JobID, entry.Attempt, entry.SubmittedBy)
		if entry.LastError != nil {
			fmt.Printf("  last_error: %s\n", *entry.LastError)
		}
		if entry.Stderr != nil {
			fmt.Printf("  stderr: %s\n", *entry.Stderr)
		}
		if *includeResult && len(entry.Result) > 0 {
			fmt.Printf("  result: %s\n", string(entry.Result))
		}
	}

	return 0
}

func parseJobStatusFlag(raw string) (queue.Status, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pending":
		return queue.StatusQueued, true
	case "ok":
		return queue.StatusSucceeded, true
	case "error":
		return queue.StatusFailed, true
	case string(queue.StatusQueued), string(queue.StatusRunning), string(queue.StatusSucceeded), string(queue.StatusFailed), string(queue.StatusTimedOut), string(queue.StatusDead):
		return queue.Status(strings.ToLower(strings.TrimSpace(raw))), true
	default:
		return "", false
	}
}

func parseTimeFlag(raw string) (time.Time, error) {
	layouts := []string{time.RFC3339Nano, time.RFC3339}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time")
}
