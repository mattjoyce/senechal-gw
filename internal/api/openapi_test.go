package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mattjoyce/ductile/internal/plugin"
)

func TestBuildOpenAPIDoc_Empty(t *testing.T) {
	doc := buildOpenAPIDoc(map[string]*plugin.Plugin{})

	if doc["openapi"] != "3.1.0" {
		t.Errorf("expected openapi 3.1.0, got %v", doc["openapi"])
	}
	paths := doc["paths"].(map[string]any)
	if len(paths) != 0 {
		t.Errorf("expected empty paths, got %d", len(paths))
	}
}

func TestBuildOpenAPIDoc_SinglePlugin(t *testing.T) {
	plugins := map[string]*plugin.Plugin{
		"echo": {
			Name: "echo",
			Commands: plugin.Commands{
				{Name: "poll", Type: plugin.CommandTypeWrite, Description: "Poll for data"},
				{Name: "health", Type: plugin.CommandTypeRead},
			},
		},
	}

	doc := buildOpenAPIDoc(plugins)

	paths := doc["paths"].(map[string]any)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}

	pollPath, ok := paths["/plugin/echo/poll"].(map[string]any)
	if !ok {
		t.Fatal("expected /plugin/echo/poll path")
	}
	post := pollPath["post"].(map[string]any)
	if post["operationId"] != "echo__poll" {
		t.Errorf("expected operationId echo__poll, got %v", post["operationId"])
	}
	if post["summary"] != "Poll for data" {
		t.Errorf("expected summary 'Poll for data', got %v", post["summary"])
	}

	healthPath, ok := paths["/plugin/echo/health"].(map[string]any)
	if !ok {
		t.Fatal("expected /plugin/echo/health path")
	}
	healthPost := healthPath["post"].(map[string]any)
	if healthPost["summary"] != "echo: health" {
		t.Errorf("expected default summary 'echo: health', got %v", healthPost["summary"])
	}
}

func TestBuildOpenAPIDoc_InputSchemaExpanded(t *testing.T) {
	plugins := map[string]*plugin.Plugin{
		"echo": {
			Name: "echo",
			Commands: plugin.Commands{
				{
					Name: "poll",
					InputSchema: map[string]any{
						"msg": "string",
					},
				},
			},
		},
	}

	doc := buildOpenAPIDoc(plugins)
	paths := doc["paths"].(map[string]any)
	post := paths["/plugin/echo/poll"].(map[string]any)["post"].(map[string]any)

	rb, ok := post["requestBody"].(map[string]any)
	if !ok {
		t.Fatal("expected requestBody")
	}
	if rb["required"] != false {
		t.Errorf("expected required false, got %v", rb["required"])
	}
	content := rb["content"].(map[string]any)
	schema := content["application/json"].(map[string]any)["schema"].(map[string]any)
	if schema["type"] != "object" {
		t.Errorf("expected expanded schema type object, got %v", schema["type"])
	}
}

func TestBuildOpenAPIDoc_NoInputSchema_NoRequestBody(t *testing.T) {
	plugins := map[string]*plugin.Plugin{
		"echo": {
			Name: "echo",
			Commands: plugin.Commands{
				{Name: "health"},
			},
		},
	}

	doc := buildOpenAPIDoc(plugins)
	paths := doc["paths"].(map[string]any)
	post := paths["/plugin/echo/health"].(map[string]any)["post"].(map[string]any)

	if _, ok := post["requestBody"]; ok {
		t.Error("expected no requestBody when no input_schema")
	}
}

func TestBuildOpenAPIDoc_SecurityScheme(t *testing.T) {
	doc := buildOpenAPIDoc(map[string]*plugin.Plugin{})

	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatal("expected components")
	}
	schemes := components["securitySchemes"].(map[string]any)
	bearer := schemes["BearerAuth"].(map[string]any)
	if bearer["type"] != "http" || bearer["scheme"] != "bearer" {
		t.Errorf("unexpected BearerAuth scheme: %v", bearer)
	}
}

func TestBuildOpenAPIDoc_MultiPlugin_PathsSorted(t *testing.T) {
	plugins := map[string]*plugin.Plugin{
		"zephyr": {Name: "zephyr", Commands: plugin.Commands{{Name: "run"}}},
		"alpha":  {Name: "alpha", Commands: plugin.Commands{{Name: "run"}}},
	}
	doc := buildOpenAPIDoc(plugins)
	paths := doc["paths"].(map[string]any)

	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if _, ok := paths["/plugin/alpha/run"]; !ok {
		t.Error("expected /plugin/alpha/run")
	}
	if _, ok := paths["/plugin/zephyr/run"]; !ok {
		t.Error("expected /plugin/zephyr/run")
	}
}

func TestHandleOpenAPIPlugin_Found_NoAuth(t *testing.T) {
	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name:     "echo",
				Commands: plugin.Commands{{Name: "poll", Type: plugin.CommandTypeWrite}},
			},
			"other": {
				Name:     "other",
				Commands: plugin.Commands{{Name: "run", Type: plugin.CommandTypeWrite}},
			},
		},
	}
	server := newTestServer(&mockQueue{}, reg)

	req := httptest.NewRequest(http.MethodGet, "/plugin/echo/openapi.json", nil)
	rr := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var doc map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&doc); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	paths := doc["paths"].(map[string]any)
	if len(paths) != 1 {
		t.Errorf("expected 1 path (echo only), got %d", len(paths))
	}
	if _, ok := paths["/plugin/echo/poll"]; !ok {
		t.Error("expected /plugin/echo/poll in paths")
	}
	if _, ok := paths["/plugin/other/run"]; ok {
		t.Error("expected /plugin/other/run to be absent from single-plugin doc")
	}
}

func TestHandleOpenAPIPlugin_NotFound(t *testing.T) {
	server := newTestServer(&mockQueue{}, &mockRegistry{plugins: map[string]*plugin.Plugin{}})

	req := httptest.NewRequest(http.MethodGet, "/plugin/unknown/openapi.json", nil)
	rr := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

func TestHandleOpenAPIAll_NoAuth(t *testing.T) {
	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name:     "echo",
				Commands: plugin.Commands{{Name: "poll", Type: plugin.CommandTypeWrite}},
			},
			"withings": {
				Name:     "withings",
				Commands: plugin.Commands{{Name: "health", Type: plugin.CommandTypeRead}},
			},
		},
	}
	server := newTestServer(&mockQueue{}, reg)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rr := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var doc map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&doc); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("expected paths map in openapi doc")
	}
	if _, ok := paths["/plugin/echo/poll"]; !ok {
		t.Fatalf("expected /plugin/echo/poll in global openapi")
	}
	if _, ok := paths["/plugin/withings/health"]; !ok {
		t.Fatalf("expected /plugin/withings/health in global openapi")
	}
}
