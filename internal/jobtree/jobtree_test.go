package jobtree

import (
	"encoding/json"
	"testing"

	"github.com/mattjoyce/ductile/internal/queue"
)

func TestAggregateTerminalResultWins(t *testing.T) {
	results := []*queue.JobResult{
		{JobID: "root", StepID: "fetch", Status: queue.StatusSucceeded, Result: json.RawMessage(`{"root":1}`)},
		{JobID: "child", StepID: "report", Status: queue.StatusSucceeded, Result: json.RawMessage(`{"final":2}`)},
	}
	out := Aggregate(results, "root", map[string]struct{}{"report": {}})

	if out.Status != string(queue.StatusSucceeded) {
		t.Fatalf("Status = %q, want succeeded", out.Status)
	}
	if string(out.FinalResult) != `{"final":2}` {
		t.Fatalf("FinalResult = %s, want terminal step result", out.FinalResult)
	}
	if len(out.Tree) != 2 {
		t.Fatalf("Tree len = %d, want 2", len(out.Tree))
	}
}

func TestAggregateFallsBackToRootWhenNoTerminal(t *testing.T) {
	results := []*queue.JobResult{
		{JobID: "root", StepID: "only", Status: queue.StatusSucceeded, Result: json.RawMessage(`{"root":1}`)},
	}
	out := Aggregate(results, "root", map[string]struct{}{"absent": {}})

	if string(out.FinalResult) != `{"root":1}` {
		t.Fatalf("FinalResult = %s, want root result fallback", out.FinalResult)
	}
}

func TestAggregateFailurePropagates(t *testing.T) {
	results := []*queue.JobResult{
		{JobID: "root", StepID: "a", Status: queue.StatusSucceeded},
		{JobID: "child", StepID: "b", Status: queue.StatusFailed},
	}
	out := Aggregate(results, "root", nil)

	if out.Status != string(queue.StatusFailed) {
		t.Fatalf("Status = %q, want failed", out.Status)
	}
}
