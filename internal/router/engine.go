package router

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"strings"

	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/router/dsl"
)

// Router is a concrete routing engine backed by compiled pipeline DSL.
type Router struct {
	set          *dsl.Set
	triggerIndex map[string][]string
	successors   map[string]map[string][]string // pipeline -> from node -> to nodes
	logger       *slog.Logger
}

var _ Engine = (*Router)(nil)

// LoadFromConfigDir loads pipelines from <configDir>/pipelines and builds a Router.
func LoadFromConfigDir(configDir string, registry *plugin.Registry, logger *slog.Logger) (Engine, error) {
	set, err := dsl.LoadAndCompileDir(configDir)
	if err != nil {
		return nil, err
	}
	if registry != nil {
		if err := validateUsesNodesExist(set, registry); err != nil {
			return nil, err
		}
	}
	return New(set, logger), nil
}

// New creates a Router from a compiled pipeline set.
func New(set *dsl.Set, logger *slog.Logger) *Router {
	if set == nil {
		set = &dsl.Set{Pipelines: map[string]*dsl.Pipeline{}}
	}
	if logger == nil {
		logger = slog.Default()
	}

	r := &Router{
		set:          set,
		triggerIndex: make(map[string][]string),
		successors:   make(map[string]map[string][]string),
		logger:       logger.With("component", "router"),
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

// PipelineCount returns the number of loaded pipelines.
func (r *Router) PipelineCount() int {
	if r.set == nil {
		return 0
	}
	return len(r.set.Pipelines)
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

	r.logger.Debug("resolving routes for event", "type", eventType, "source_plugin", req.SourcePlugin)

	var out []Dispatch

	// Intra-pipeline transitions: if current step/pipeline are known, walk outgoing edges.
	if req.SourcePipeline != "" && req.SourceStepID != "" {
		next := r.successors[req.SourcePipeline][req.SourceStepID]
		if len(next) > 0 {
			r.logger.Debug("matched intra-pipeline transition",
				"pipeline", req.SourcePipeline,
				"from_step", req.SourceStepID,
				"next_steps", len(next))

			for _, nodeID := range next {
				dispatches, err := r.resolveNodeDispatches(req.SourcePipeline, nodeID, req)
				if err != nil {
					return nil, err
				}
				out = append(out, dispatches...)
			}
		}
	}

	// Root triggers: event type can start one or more pipelines.
	if pipelines, ok := r.triggerIndex[eventType]; ok {
		r.logger.Debug("matched root trigger pipelines", "event_type", eventType, "count", len(pipelines))
		for _, pipelineName := range pipelines {
			pipeline := r.set.Pipelines[pipelineName]
			if pipeline == nil {
				continue
			}
			r.logger.Info("triggering pipeline", "name", pipelineName, "event_type", eventType)
			for _, nodeID := range pipeline.EntryNodeIDs {
				dispatches, err := r.resolveNodeDispatches(pipelineName, nodeID, req)
				if err != nil {
					return nil, err
				}
				out = append(out, dispatches...)
			}
		}
	}

	if len(out) > 0 {
		r.logger.Info("routing event resulting in child jobs", "type", eventType, "jobs", len(out))
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
		maps.Copy(cloned.Payload, ev.Payload)
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

// GetPipelineByTrigger returns the first pipeline matched by a trigger event.
func (r *Router) GetPipelineByTrigger(trigger string) *PipelineInfo {
	r.logger.Debug("looking up pipeline by trigger", "trigger", trigger)
	pipelines, ok := r.triggerIndex[trigger]
	if !ok || len(pipelines) == 0 {
		return nil
	}

	// For now, return the first one found.
	name := pipelines[0]
	pipeline := r.set.Pipelines[name]
	if pipeline == nil {
		return nil
	}

	return &PipelineInfo{
		Name:            pipeline.Name,
		Trigger:         pipeline.Trigger,
		EntryStepID:     firstEntryStepID(pipeline.EntryNodeIDs),
		TerminalStepIDs: append([]string(nil), pipeline.TerminalNodeIDs...),
		ExecutionMode:   pipeline.ExecutionMode,
		Timeout:         pipeline.Timeout,
	}
}

// GetPipelineByName returns info about a pipeline by its name.
func (r *Router) GetPipelineByName(name string) *PipelineInfo {
	r.logger.Debug("looking up pipeline by name", "name", name)
	pipeline, ok := r.set.Pipelines[name]
	if !ok {
		return nil
	}

	return &PipelineInfo{
		Name:            pipeline.Name,
		Trigger:         pipeline.Trigger,
		EntryStepID:     firstEntryStepID(pipeline.EntryNodeIDs),
		TerminalStepIDs: append([]string(nil), pipeline.TerminalNodeIDs...),
		ExecutionMode:   pipeline.ExecutionMode,
		Timeout:         pipeline.Timeout,
	}
}

// GetEntryDispatches returns the initial jobs to enqueue when a pipeline is explicitly triggered.
func (r *Router) GetEntryDispatches(pipelineName string, event protocol.Event) ([]Dispatch, error) {
	pipeline, ok := r.set.Pipelines[pipelineName]
	if !ok {
		return nil, fmt.Errorf("pipeline %q not found", pipelineName)
	}

	var out []Dispatch
	// Create a dummy request for resolution context
	req := Request{
		Event: event,
	}

	for _, nodeID := range pipeline.EntryNodeIDs {
		dispatches, err := r.resolveNodeDispatches(pipelineName, nodeID, req)
		if err != nil {
			return nil, err
		}
		out = append(out, dispatches...)
	}

	return out, nil
}

// PipelineSummary returns info about all loaded pipelines.
func (r *Router) PipelineSummary() []PipelineInfo {
	var out []PipelineInfo
	for name, pipeline := range r.set.Pipelines {
		out = append(out, PipelineInfo{
			Name:            name,
			Trigger:         pipeline.Trigger,
			EntryStepID:     firstEntryStepID(pipeline.EntryNodeIDs),
			TerminalStepIDs: append([]string(nil), pipeline.TerminalNodeIDs...),
			ExecutionMode:   pipeline.ExecutionMode,
			Timeout:         pipeline.Timeout,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func firstEntryStepID(entryNodeIDs []string) string {
	if len(entryNodeIDs) == 0 {
		return ""
	}
	return entryNodeIDs[0]
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
