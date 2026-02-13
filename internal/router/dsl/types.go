package dsl

import "time"

// FileSpec is one YAML file containing one or more pipelines.
type FileSpec struct {
	Pipelines []PipelineSpec `yaml:"pipelines"`
}

// PipelineSpec defines a single pipeline entry in YAML.
type PipelineSpec struct {
	Name          string        `yaml:"name"`
	On            string        `yaml:"on"`
	Steps         []StepSpec    `yaml:"steps"`
	ExecutionMode string        `yaml:"execution_mode,omitempty"`
	Timeout       time.Duration `yaml:"timeout,omitempty"`
}

// StepSpec is one DSL step. Exactly one execution field must be set:
// - uses
// - call
// - steps
// - split
type StepSpec struct {
	ID    string     `yaml:"id,omitempty"`
	Uses  string     `yaml:"uses,omitempty"`
	Call  string     `yaml:"call,omitempty"`
	Steps []StepSpec `yaml:"steps,omitempty"`
	Split []StepSpec `yaml:"split,omitempty"`
}

// NodeKind identifies the executable action represented by a DAG node.
type NodeKind string

const (
	NodeKindUses NodeKind = "uses"
	NodeKindCall NodeKind = "call"
)

// Node is one executable vertex in a compiled pipeline DAG.
type Node struct {
	ID   string
	Kind NodeKind
	Uses string
	Call string
}

// Edge defines a directed dependency between two nodes.
type Edge struct {
	From string
	To   string
}

// Pipeline is a compiled DAG for one named pipeline.
type Pipeline struct {
	Name            string
	Trigger         string
	ExecutionMode   string
	Timeout         time.Duration
	Nodes           map[string]Node
	Edges           []Edge
	EntryNodeIDs    []string
	TerminalNodeIDs []string
	CalledPipelines []string
	Fingerprint     string // blake3:<hex> of normalized compiled form.
}

// Set is a compiled collection of pipelines keyed by name.
type Set struct {
	Pipelines map[string]*Pipeline
}
