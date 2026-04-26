package api

import (
	"testing"

	"github.com/mattjoyce/ductile/internal/protocol"
	"github.com/mattjoyce/ductile/internal/router"
	"github.com/mattjoyce/ductile/internal/router/conditions"
	"github.com/mattjoyce/ductile/internal/router/dsl"
)

// renderCompiledRoutesRouter is a tiny PipelineRouter shim for renderCompiledRoutes.
// We only need GetCompiledRoutes; the other interface methods are stubs.
type renderCompiledRoutesRouter struct {
	routes map[string][]dsl.CompiledRoute
}

func (r *renderCompiledRoutesRouter) GetPipelineByTrigger(string) *router.PipelineInfo { return nil }
func (r *renderCompiledRoutesRouter) GetPipelineByName(string) *router.PipelineInfo    { return nil }
func (r *renderCompiledRoutesRouter) GetEntryDispatches(string, protocol.Event) ([]router.Dispatch, error) {
	return nil, nil
}
func (r *renderCompiledRoutesRouter) GetCompiledRoutes(name string) []dsl.CompiledRoute {
	return r.routes[name]
}
func (r *renderCompiledRoutesRouter) GetNode(string, string) (dsl.Node, bool) {
	return dsl.Node{}, false
}
func (r *renderCompiledRoutesRouter) PipelineSummary() []router.PipelineInfo {
	out := make([]router.PipelineInfo, 0, len(r.routes))
	for name := range r.routes {
		out = append(out, router.PipelineInfo{Name: name})
	}
	return out
}

// TestRenderCompiledRoutesExposesSprint17Selectors asserts that the inspection
// surface surfaces from_plugin: and pipeline-level if: so the richer match
// shape is visible to operators rather than implicit.
func TestRenderCompiledRoutesExposesSprint17Selectors(t *testing.T) {
	set, err := dsl.CompileSpecs([]dsl.PipelineSpec{{
		Name:       "page-on-claude-failure",
		OnHook:     "job.failed",
		FromPlugin: "claude_harvest",
		If: &conditions.Condition{
			Path:  "context.severity",
			Op:    conditions.OpEq,
			Value: "high",
		},
		Steps: []dsl.StepSpec{{ID: "page", Uses: "pagerduty"}},
	}})
	if err != nil {
		t.Fatalf("CompileSpecs: %v", err)
	}
	pipeline := set.Pipelines["page-on-claude-failure"]
	shim := &renderCompiledRoutesRouter{
		routes: map[string][]dsl.CompiledRoute{
			pipeline.Name: pipeline.CompiledRoutes,
		},
	}

	rendered := renderCompiledRoutes(shim)
	got, ok := rendered[pipeline.Name]
	if !ok {
		t.Fatalf("rendered output missing pipeline %q", pipeline.Name)
	}

	var sawHookEntry bool
	for _, route := range got {
		if route.Source.HookSignal == "" {
			continue
		}
		sawHookEntry = true
		if route.Source.SourcePlugin != "claude_harvest" {
			t.Fatalf("rendered source_plugin = %q, want %q", route.Source.SourcePlugin, "claude_harvest")
		}
		if route.Source.If == nil {
			t.Fatalf("rendered hook entry route missing if predicate")
		}
		if route.Source.If.Path != "context.severity" {
			t.Fatalf("rendered if path = %q, want context.severity", route.Source.If.Path)
		}
	}
	if !sawHookEntry {
		t.Fatalf("expected at least one hook entry route in rendered output")
	}
}

// TestRenderCompiledRoutesEmptyForNoRouter exercises the nil-router path.
func TestRenderCompiledRoutesEmptyForNoRouter(t *testing.T) {
	out := renderCompiledRoutes(nil)
	if out != nil {
		t.Fatalf("expected nil for nil router, got %+v", out)
	}
}

// TestRenderCompiledRoutesSkipsPipelinesWithNoRoutes ensures empty entries
// do not appear in the output.
func TestRenderCompiledRoutesSkipsPipelinesWithNoRoutes(t *testing.T) {
	shim := &renderCompiledRoutesRouter{routes: map[string][]dsl.CompiledRoute{}}
	out := renderCompiledRoutes(shim)
	if len(out) != 0 {
		t.Fatalf("expected empty output, got %+v", out)
	}
}
