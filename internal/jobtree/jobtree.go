// Package jobtree computes the outcome of a settled job tree.
//
// This is deliberately a pure function over values: a slice of completed
// job results in, an Outcome value out. It carries no knowledge of HTTP,
// of the local API, or of remote relay. Both the synchronous pipeline API
// handler and the remote relay receiver feed it the same data and read the
// same answer, so the two transports cannot drift in how they decide
// "did this pipeline succeed, and what did it produce".
package jobtree

import (
	"encoding/json"
	"time"

	"github.com/mattjoyce/ductile/internal/queue"
)

// Node is one job's contribution to a settled tree.
type Node struct {
	JobID       string          `json:"job_id"`
	ParentJobID *string         `json:"parent_job_id,omitempty"`
	Plugin      string          `json:"plugin"`
	Command     string          `json:"command"`
	Status      string          `json:"status"`
	Result      json.RawMessage `json:"result,omitempty"`
	LastError   *string         `json:"last_error,omitempty"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// Outcome is the rolled-up answer for a settled tree.
type Outcome struct {
	// Status is "succeeded" unless any job in the tree failed, timed out, or
	// died — in which case it reflects that non-success terminal status.
	Status string
	// FinalResult is the terminal step's result when the pipeline declares
	// terminal steps, otherwise the root job's result.
	FinalResult json.RawMessage
	// Tree is every job in the settled tree.
	Tree []Node
}

// Aggregate folds a settled set of job results into a single Outcome.
//
// rootJobID identifies the root of the tree (its result is the fallback
// FinalResult). terminalSteps is the set of step IDs the pipeline declares
// terminal; the last matching job's result wins as FinalResult.
func Aggregate(results []*queue.JobResult, rootJobID string, terminalSteps map[string]struct{}) Outcome {
	out := Outcome{Status: string(queue.StatusSucceeded)}

	var rootResult json.RawMessage
	var terminalResult json.RawMessage
	haveTerminal := false

	for _, res := range results {
		if res == nil {
			continue
		}
		if res.JobID == rootJobID {
			rootResult = res.Result
		}
		// Any non-success terminal status makes the whole tree non-success.
		switch res.Status {
		case queue.StatusFailed, queue.StatusTimedOut, queue.StatusDead:
			out.Status = string(res.Status)
		}
		if _, ok := terminalSteps[res.StepID]; ok {
			terminalResult = res.Result
			haveTerminal = true
		}
		out.Tree = append(out.Tree, Node{
			JobID:       res.JobID,
			ParentJobID: res.ParentJobID,
			Plugin:      res.Plugin,
			Command:     res.Command,
			Status:      string(res.Status),
			Result:      res.Result,
			LastError:   res.LastError,
			StartedAt:   res.StartedAt,
			CompletedAt: res.CompletedAt,
		})
	}

	if haveTerminal {
		out.FinalResult = terminalResult
	} else {
		out.FinalResult = rootResult
	}
	return out
}
