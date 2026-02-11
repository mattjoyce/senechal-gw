package router

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mattjoyce/senechal-gw/internal/plugin"
	"github.com/mattjoyce/senechal-gw/internal/protocol"
	"github.com/mattjoyce/senechal-gw/internal/router/dsl"
)

// Router is a concrete routing engine backed by compiled pipeline DSL.
type Router struct {
	set          *dsl.Set
	triggerIndex map[string][]string
	successors   map[string]map[string][]string // pipeline -> from node -> to nodes
}

var _ Engine = (*Router)(nil)

// LoadFromConfigDir loads pipelines from <configDir>/pipelines and builds a Router.
func LoadFromConfigDir(configDir string, registry *plugin.Registry) (*Router, error) {
	set, err := dsl.LoadAndCompileDir(configDir)
	if err != nil {
		return nil, err
	}
	if registry != nil {
		if err := validateUsesNodesExist(set, registry); err != nil {
			return nil, err
		}
	}
	return New(set), nil
}

// New creates a Router from a compiled pipeline set.
func New(set *dsl.Set) *Router {
	if set == nil {
		set = &dsl.Set{Pipelines: map[string]*dsl.Pipeline{}}
	}

	r := &Router{
		set:          set,
		triggerIndex: make(map[string][]string),
		successors:   make(map[string]map[string][]string),
	}

	for name, pipeline := range set.Pipelines {
		r.triggerIndex[pipeline.Trigger] = append(r.triggerIndex[pipeline.Trigger], name)

		succ := make(map[string][]string)
		for _, edge := range pipeline.Edges {
			succ[edge.From] = append(succ[edge.From], edge.To)
		}
		for nodeID := range succ {
			sort.Strings(succ[nodeID])
		}
		r.successors[name] = succ
	}
	for trigger := range r.triggerIndex {
		sort.Strings(r.triggerIndex[trigger])
	}

	return r
}

// Next resolves downstream dispatches for one emitted event.
func (r *Router) Next(ctx context.Context, req Request) ([]Dispatch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	eventType := strings.TrimSpace(req.Event.Type)
	if eventType == "" {
		return nil, fmt.Errorf("event.type is required")
	}

	var out []Dispatch

	// Intra-pipeline transitions: if current step/pipeline are known, walk outgoing edges.
	if req.SourcePipeline != "" && req.SourceStepID != "" {
		next := r.successors[req.SourcePipeline][req.SourceStepID]
		for _, nodeID := range next {
			dispatches, err := r.resolveNodeDispatches(req.SourcePipeline, nodeID, req)
			if err != nil {
				return nil, err
			}
			out = append(out, dispatches...)
		}
	}

	// Root triggers: event type can start one or more pipelines.
	for _, pipelineName := range r.triggerIndex[eventType] {
		pipeline := r.set.Pipelines[pipelineName]
		if pipeline == nil {
			continue
		}
		for _, nodeID := range pipeline.EntryNodeIDs {
			dispatches, err := r.resolveNodeDispatches(pipelineName, nodeID, req)
			if err != nil {
				return nil, err
			}
			out = append(out, dispatches...)
		}
	}

	return dedupeDispatches(out), nil
}

func (r *Router) resolveNodeDispatches(pipelineName, nodeID string, req Request) ([]Dispatch, error) {
	pipeline := r.set.Pipelines[pipelineName]
	if pipeline == nil {
		return nil, fmt.Errorf("unknown pipeline %q", pipelineName)
	}
	node, ok := pipeline.Nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("pipeline %q node %q not found", pipelineName, nodeID)
	}

	switch node.Kind {
	case dsl.NodeKindUses:
		return []Dispatch{
			{
				Plugin:          node.Uses,
				Command:         "handle",
				Event:           cloneEvent(req.Event),
				PipelineName:    pipelineName,
				StepID:          nodeID,
				ParentJobID:     req.SourceJobID,
				ParentContextID: req.SourceContextID,
				SourceEventID:   req.SourceEventID,
			},
		}, nil

	case dsl.NodeKindCall:
		called := strings.TrimSpace(node.Call)
		if called == "" {
			return nil, fmt.Errorf("pipeline %q node %q has empty call target", pipelineName, nodeID)
		}
		target := r.set.Pipelines[called]
		if target == nil {
			return nil, fmt.Errorf("pipeline %q node %q references unknown pipeline %q", pipelineName, nodeID, called)
		}
		var out []Dispatch
		for _, entryID := range target.EntryNodeIDs {
			dispatches, err := r.resolveNodeDispatches(called, entryID, req)
			if err != nil {
				return nil, err
			}
			out = append(out, dispatches...)
		}
		return out, nil
	}

	return nil, fmt.Errorf("unsupported node kind %q", node.Kind)
}

func cloneEvent(ev protocol.Event) protocol.Event {
	cloned := protocol.Event{
		Type:      ev.Type,
		DedupeKey: ev.DedupeKey,
		Source:    ev.Source,
		Timestamp: ev.Timestamp,
		EventID:   ev.EventID,
	}
	if ev.Payload != nil {
		cloned.Payload = make(map[string]any, len(ev.Payload))
		for k, v := range ev.Payload {
			cloned.Payload[k] = v
		}
	}
	return cloned
}

func dedupeDispatches(in []Dispatch) []Dispatch {
	seen := make(map[string]struct{}, len(in))
	out := make([]Dispatch, 0, len(in))
	for _, d := range in {
		key := d.PipelineName + "\x00" + d.StepID + "\x00" + d.Plugin + "\x00" + d.Command + "\x00" + d.SourceEventID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, d)
	}
	return out
}

func validateUsesNodesExist(set *dsl.Set, registry *plugin.Registry) error {
	for pipelineName, pipeline := range set.Pipelines {
		for _, node := range pipeline.Nodes {
			if node.Kind != dsl.NodeKindUses {
				continue
			}
			if _, ok := registry.Get(node.Uses); !ok {
				return fmt.Errorf("pipeline %q step %q references unknown plugin %q", pipelineName, node.ID, node.Uses)
			}
		}
	}
	return nil
}
