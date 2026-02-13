package dsl

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zeebo/blake3"
)

// CompileSpecs compiles pipeline DSL definitions into validated DAGs.
func CompileSpecs(specs []PipelineSpec) (*Set, error) {
	out := &Set{Pipelines: make(map[string]*Pipeline)}

	for i, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			return nil, fmt.Errorf("pipelines[%d]: name is required", i)
		}
		if _, exists := out.Pipelines[name]; exists {
			return nil, fmt.Errorf("duplicate pipeline name %q", name)
		}

		compiled, err := compilePipeline(spec)
		if err != nil {
			return nil, fmt.Errorf("pipeline %q: %w", name, err)
		}
		out.Pipelines[name] = compiled
	}

	if err := validatePipelineCalls(out.Pipelines); err != nil {
		return nil, err
	}

	return out, nil
}

func compilePipeline(spec PipelineSpec) (*Pipeline, error) {
	name := strings.TrimSpace(spec.Name)
	trigger := strings.TrimSpace(spec.On)
	if trigger == "" {
		return nil, fmt.Errorf("on is required")
	}
	if len(spec.Steps) == 0 {
		return nil, fmt.Errorf("steps must be non-empty")
	}

	pipeline := &Pipeline{
		Name:          name,
		Trigger:       trigger,
		ExecutionMode: spec.ExecutionMode,
		Timeout:       spec.Timeout,
		Nodes:         make(map[string]Node),
	}
	if pipeline.ExecutionMode == "" {
		pipeline.ExecutionMode = "async"
	}
	if pipeline.Timeout == 0 {
		pipeline.Timeout = 30 * time.Second
	}
	builder := compileBuilder{
		pipeline: pipeline,
		called:   make(map[string]struct{}),
	}

	entry, terminal, err := builder.compileSteps(spec.Steps)
	if err != nil {
		return nil, err
	}
	pipeline.EntryNodeIDs = sortedUnique(entry)
	pipeline.TerminalNodeIDs = sortedUnique(terminal)
	pipeline.CalledPipelines = sortedMapKeys(builder.called)
	sortEdges(pipeline.Edges)

	if err := validatePipelineDAG(pipeline); err != nil {
		return nil, err
	}

	fingerprint, err := fingerprintPipeline(pipeline)
	if err != nil {
		return nil, err
	}
	pipeline.Fingerprint = fingerprint
	return pipeline, nil
}

type compileBuilder struct {
	pipeline *Pipeline
	nextAuto int
	called   map[string]struct{}
}

func (b *compileBuilder) compileSteps(steps []StepSpec) (entry []string, terminal []string, err error) {
	if len(steps) == 0 {
		return nil, nil, fmt.Errorf("steps must be non-empty")
	}

	var previous []string
	for i, step := range steps {
		stepEntry, stepTerminal, err := b.compileStep(step)
		if err != nil {
			return nil, nil, fmt.Errorf("steps[%d]: %w", i, err)
		}
		if len(stepEntry) == 0 || len(stepTerminal) == 0 {
			return nil, nil, fmt.Errorf("steps[%d]: compiled to empty node set", i)
		}

		if i == 0 {
			entry = append(entry, stepEntry...)
		}
		if len(previous) > 0 {
			b.addEdges(previous, stepEntry)
		}
		previous = stepTerminal
	}

	return entry, previous, nil
}

func (b *compileBuilder) compileStep(step StepSpec) (entry []string, terminal []string, err error) {
	modeCount := 0
	if strings.TrimSpace(step.Uses) != "" {
		modeCount++
	}
	if strings.TrimSpace(step.Call) != "" {
		modeCount++
	}
	if len(step.Steps) > 0 {
		modeCount++
	}
	if len(step.Split) > 0 {
		modeCount++
	}
	if modeCount != 1 {
		return nil, nil, fmt.Errorf("step must define exactly one of uses, call, steps, or split")
	}

	switch {
	case strings.TrimSpace(step.Uses) != "":
		id, err := b.allocNodeID(step.ID)
		if err != nil {
			return nil, nil, err
		}
		node := Node{
			ID:   id,
			Kind: NodeKindUses,
			Uses: strings.TrimSpace(step.Uses),
		}
		b.pipeline.Nodes[id] = node
		return []string{id}, []string{id}, nil

	case strings.TrimSpace(step.Call) != "":
		id, err := b.allocNodeID(step.ID)
		if err != nil {
			return nil, nil, err
		}
		callTarget := strings.TrimSpace(step.Call)
		node := Node{
			ID:   id,
			Kind: NodeKindCall,
			Call: callTarget,
		}
		b.pipeline.Nodes[id] = node
		b.called[callTarget] = struct{}{}
		return []string{id}, []string{id}, nil

	case len(step.Steps) > 0:
		return b.compileSteps(step.Steps)

	case len(step.Split) > 0:
		var allEntry []string
		var allTerminal []string
		for i, branch := range step.Split {
			branchEntry, branchTerminal, err := b.compileStep(branch)
			if err != nil {
				return nil, nil, fmt.Errorf("split[%d]: %w", i, err)
			}
			allEntry = append(allEntry, branchEntry...)
			allTerminal = append(allTerminal, branchTerminal...)
		}
		return sortedUnique(allEntry), sortedUnique(allTerminal), nil
	}

	return nil, nil, fmt.Errorf("unreachable step state")
}

func (b *compileBuilder) allocNodeID(preferred string) (string, error) {
	id := strings.TrimSpace(preferred)
	if id == "" {
		for {
			b.nextAuto++
			candidate := fmt.Sprintf("step_%d", b.nextAuto)
			if _, exists := b.pipeline.Nodes[candidate]; !exists {
				id = candidate
				break
			}
		}
	}
	if _, exists := b.pipeline.Nodes[id]; exists {
		return "", fmt.Errorf("duplicate step id %q", id)
	}
	return id, nil
}

func (b *compileBuilder) addEdges(fromNodes, toNodes []string) {
	for _, from := range fromNodes {
		for _, to := range toNodes {
			b.pipeline.Edges = append(b.pipeline.Edges, Edge{From: from, To: to})
		}
	}
}

func validatePipelineCalls(pipelines map[string]*Pipeline) error {
	for name, pipeline := range pipelines {
		for _, called := range pipeline.CalledPipelines {
			if _, ok := pipelines[called]; !ok {
				return fmt.Errorf("pipeline %q references unknown pipeline %q", name, called)
			}
		}
	}

	state := make(map[string]int)
	var walk func(name string, stack []string) error
	walk = func(name string, stack []string) error {
		switch state[name] {
		case 2:
			return nil
		case 1:
			idx := 0
			for i := range stack {
				if stack[i] == name {
					idx = i
					break
				}
			}
			cycle := append(append([]string{}, stack[idx:]...), name)
			return fmt.Errorf("pipeline call cycle detected: %s", strings.Join(cycle, " -> "))
		}

		state[name] = 1
		stack = append(stack, name)
		for _, dep := range pipelines[name].CalledPipelines {
			if err := walk(dep, stack); err != nil {
				return err
			}
		}
		state[name] = 2
		return nil
	}

	names := make([]string, 0, len(pipelines))
	for name := range pipelines {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if err := walk(name, nil); err != nil {
			return err
		}
	}
	return nil
}

func validatePipelineDAG(p *Pipeline) error {
	inDegree := make(map[string]int, len(p.Nodes))
	adj := make(map[string][]string, len(p.Nodes))

	for id := range p.Nodes {
		inDegree[id] = 0
	}
	for _, edge := range p.Edges {
		if _, ok := p.Nodes[edge.From]; !ok {
			return fmt.Errorf("edge references unknown from node %q", edge.From)
		}
		if _, ok := p.Nodes[edge.To]; !ok {
			return fmt.Errorf("edge references unknown to node %q", edge.To)
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
		inDegree[edge.To]++
	}

	queue := make([]string, 0, len(p.Nodes))
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	visited := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		visited++

		for _, next := range adj[n] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if visited != len(p.Nodes) {
		return fmt.Errorf("compiled pipeline graph contains a cycle")
	}
	return nil
}

func fingerprintPipeline(p *Pipeline) (string, error) {
	type fingerprintShape struct {
		Name            string   `json:"name"`
		Trigger         string   `json:"trigger"`
		Nodes           []Node   `json:"nodes"`
		Edges           []Edge   `json:"edges"`
		EntryNodeIDs    []string `json:"entry_node_ids"`
		TerminalNodeIDs []string `json:"terminal_node_ids"`
		CalledPipelines []string `json:"called_pipelines"`
	}

	nodes := make([]Node, 0, len(p.Nodes))
	for _, node := range p.Nodes {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	shape := fingerprintShape{
		Name:            p.Name,
		Trigger:         p.Trigger,
		Nodes:           nodes,
		Edges:           append([]Edge(nil), p.Edges...),
		EntryNodeIDs:    append([]string(nil), p.EntryNodeIDs...),
		TerminalNodeIDs: append([]string(nil), p.TerminalNodeIDs...),
		CalledPipelines: append([]string(nil), p.CalledPipelines...),
	}
	sortEdges(shape.Edges)
	sort.Strings(shape.EntryNodeIDs)
	sort.Strings(shape.TerminalNodeIDs)
	sort.Strings(shape.CalledPipelines)

	body, err := json.Marshal(shape)
	if err != nil {
		return "", fmt.Errorf("marshal pipeline fingerprint input: %w", err)
	}
	sum := blake3.Sum256(body)
	return "blake3:" + hex.EncodeToString(sum[:]), nil
}

func sortEdges(edges []Edge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})
}

func sortedUnique(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	for _, value := range in {
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortedMapKeys(in map[string]struct{}) []string {
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
