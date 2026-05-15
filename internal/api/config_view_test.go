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

// TestSanitizePluginConfigRecursive is the F-006 regression: sensitive
// values must be redacted at every nesting depth, including under a
// non-sensitive parent key, inside nested maps, and in list elements.
// The shallow (pre-fix) implementation passed nested maps through
// unchanged, so this test genuinely requires the recursive walk.
func TestSanitizePluginConfigRecursive(t *testing.T) {
	if got := sanitizePluginConfig(nil); got != nil {
		t.Fatalf("nil input must return nil, got %+v", got)
	}

	in := map[string]any{
		"endpoint": "https://example.test", // non-sensitive scalar
		"api_key":  "TOP_SECRET",           // top-level sensitive
		"redaction_fixture": map[string]any{ // non-sensitive parent
			"nested_token": "LEAK_NESTED_TOKEN", // sensitive at depth 2
			"inner": map[string]any{
				"password": "LEAK_DEEP_PASSWORD", // sensitive at depth 3
				"region":   "us-east-1",          // preserved
			},
			"list": []any{
				map[string]any{"api_key": "LEAK_LIST_API_KEY"}, // in slice
				"plain-element",
			},
		},
		"strmap": map[string]string{
			"secret": "LEAK_STRMAP_SECRET", // sensitive in map[string]string
			"name":   "keep-me",
		},
	}
	out := sanitizePluginConfig(in)

	if out["endpoint"] != "https://example.test" {
		t.Errorf("non-sensitive scalar altered: %v", out["endpoint"])
	}
	if out["api_key"] != redactionSentinel {
		t.Errorf("top-level sensitive not redacted: %v", out["api_key"])
	}

	rf, ok := out["redaction_fixture"].(map[string]any)
	if !ok {
		t.Fatalf("redaction_fixture not a map: %T", out["redaction_fixture"])
	}
	if rf["nested_token"] != redactionSentinel {
		t.Errorf("depth-2 sensitive leaked: %v", rf["nested_token"])
	}
	inner, ok := rf["inner"].(map[string]any)
	if !ok {
		t.Fatalf("inner not a map: %T", rf["inner"])
	}
	if inner["password"] != redactionSentinel {
		t.Errorf("depth-3 sensitive leaked: %v", inner["password"])
	}
	if inner["region"] != "us-east-1" {
		t.Errorf("non-sensitive nested value altered: %v", inner["region"])
	}
	lst, ok := rf["list"].([]any)
	if !ok {
		t.Fatalf("list not a slice: %T", rf["list"])
	}
	elem0, ok := lst[0].(map[string]any)
	if !ok {
		t.Fatalf("list[0] not a map: %T", lst[0])
	}
	if elem0["api_key"] != redactionSentinel {
		t.Errorf("sensitive in slice element leaked: %v", elem0["api_key"])
	}
	if lst[1] != "plain-element" {
		t.Errorf("non-sensitive slice element altered: %v", lst[1])
	}

	sm, ok := out["strmap"].(map[string]any)
	if !ok {
		t.Fatalf("strmap not a map: %T", out["strmap"])
	}
	if sm["secret"] != redactionSentinel {
		t.Errorf("map[string]string sensitive leaked: %v", sm["secret"])
	}
	if sm["name"] != "keep-me" {
		t.Errorf("map[string]string non-sensitive altered: %v", sm["name"])
	}

	// Input must not be mutated by sanitization.
	if in["api_key"] != "TOP_SECRET" {
		t.Errorf("input mutated: api_key now %v", in["api_key"])
	}
	origRF := in["redaction_fixture"].(map[string]any)
	if origRF["nested_token"] != "LEAK_NESTED_TOKEN" {
		t.Errorf("input nested map mutated: %v", origRF["nested_token"])
	}
}
