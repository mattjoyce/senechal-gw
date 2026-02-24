package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/router"
)

type mockSkillsRouter struct {
	mockRouter
	pipelines []router.PipelineInfo
}

func (m *mockSkillsRouter) PipelineSummary() []router.PipelineInfo {
	out := make([]router.PipelineInfo, len(m.pipelines))
	copy(out, m.pipelines)
	return out
}

func TestHandleListPlugins_NoAuth(t *testing.T) {
	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {Name: "echo", Commands: plugin.Commands{{Name: "poll"}}},
		},
	}
	server := newTestServer(&mockQueue{}, reg)

	req := httptest.NewRequest(http.MethodGet, "/plugins", nil)
	// No Authorization header — discovery must be unauthenticated.
	rr := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 without auth, got %d", rr.Code)
	}
}

func TestHandleListSkills_NoAuth(t *testing.T) {
	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {Name: "echo", Commands: plugin.Commands{{Name: "poll", Type: plugin.CommandTypeWrite}}},
		},
	}
	server := newTestServer(&mockQueue{}, reg)
	server.router = &mockSkillsRouter{
		pipelines: []router.PipelineInfo{
			{Name: "daily-summary", Trigger: "scheduler.tick", ExecutionMode: "asynchronous"},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/skills", nil)
	rr := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 without auth, got %d", rr.Code)
	}
}

func TestHandleListPlugins(t *testing.T) {
	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name:        "echo",
				Version:     "0.1.0",
				Description: "Echo plugin",
				Commands: plugin.Commands{
					{Name: "poll", Type: plugin.CommandTypeWrite},
					{Name: "health", Type: plugin.CommandTypeRead},
				},
			},
			"fabric": {
				Name:        "fabric",
				Version:     "1.0.0",
				Description: "Fabric plugin",
				Commands: plugin.Commands{
					{Name: "handle", Type: plugin.CommandTypeWrite},
				},
			},
		},
	}

	server := newTestServer(&mockQueue{}, reg)

	req := httptest.NewRequest(http.MethodGet, "/plugins", nil)
	req.Header.Set("Authorization", "Bearer test-key-123")
	rr := httptest.NewRecorder()

	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp PluginListResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(resp.Plugins))
	}

	// Results should be sorted by name
	if resp.Plugins[0].Name != "echo" {
		t.Errorf("expected first plugin to be echo, got %s", resp.Plugins[0].Name)
	}
	if resp.Plugins[1].Name != "fabric" {
		t.Errorf("expected second plugin to be fabric, got %s", resp.Plugins[1].Name)
	}

	if len(resp.Plugins[0].Commands) != 2 {
		t.Errorf("expected 2 commands for echo, got %d", len(resp.Plugins[0].Commands))
	}
}

func TestHandleListSkills(t *testing.T) {
	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name:        "echo",
				Version:     "0.1.0",
				Description: "Echo plugin",
				Commands: plugin.Commands{
					{Name: "poll", Type: plugin.CommandTypeWrite, Description: "Poll for events"},
					{Name: "health", Type: plugin.CommandTypeRead, Description: "Read health"},
				},
			},
		},
	}
	server := newTestServer(&mockQueue{}, reg)
	server.router = &mockSkillsRouter{
		pipelines: []router.PipelineInfo{
			{
				Name:          "discord-fabric",
				Trigger:       "discord.message",
				ExecutionMode: "synchronous",
				Timeout:       45 * time.Second,
			},
			{
				Name:    "nightly-summary",
				Trigger: "scheduler.tick",
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/skills", nil)
	rr := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp SkillsIndexResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Skills) != 4 {
		t.Fatalf("expected 4 skills, got %d", len(resp.Skills))
	}

	index := make(map[string]SkillSummary, len(resp.Skills))
	for _, s := range resp.Skills {
		index[s.Name] = s
	}

	health, ok := index["plugin.echo.health"]
	if !ok {
		t.Fatalf("missing plugin.echo.health skill")
	}
	if health.Kind != "plugin" || health.Tier != "READ" || health.Endpoint != "/plugin/echo/health" {
		t.Fatalf("unexpected plugin.echo.health skill: %+v", health)
	}

	poll, ok := index["plugin.echo.poll"]
	if !ok {
		t.Fatalf("missing plugin.echo.poll skill")
	}
	if poll.Kind != "plugin" || poll.Tier != "WRITE" || poll.Endpoint != "/plugin/echo/poll" {
		t.Fatalf("unexpected plugin.echo.poll skill: %+v", poll)
	}

	syncPipeline, ok := index["pipeline.discord-fabric"]
	if !ok {
		t.Fatalf("missing pipeline.discord-fabric skill")
	}
	if syncPipeline.Kind != "pipeline" || syncPipeline.ExecutionMode != "synchronous" || syncPipeline.TimeoutSecs != 45 {
		t.Fatalf("unexpected pipeline.discord-fabric skill: %+v", syncPipeline)
	}

	asyncPipeline, ok := index["pipeline.nightly-summary"]
	if !ok {
		t.Fatalf("missing pipeline.nightly-summary skill")
	}
	if asyncPipeline.ExecutionMode != "asynchronous" {
		t.Fatalf("expected default async mode, got %q", asyncPipeline.ExecutionMode)
	}
}

func TestHandleGetPlugin(t *testing.T) {
	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{"type": "string"},
		},
	}
	reg := &mockRegistry{
		plugins: map[string]*plugin.Plugin{
			"echo": {
				Name:        "echo",
				Version:     "0.1.0",
				Description: "Echo plugin",
				Protocol:    2,
				Commands: plugin.Commands{
					{
						Name:        "poll",
						Type:        plugin.CommandTypeWrite,
						Description: "Poll command",
						InputSchema: inputSchema,
					},
				},
			},
		},
	}

	server := newTestServer(&mockQueue{}, reg)

	t.Run("found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/plugin/echo", nil)
		req.Header.Set("Authorization", "Bearer test-key-123")
		rr := httptest.NewRecorder()

		server.setupRoutes().ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}

		var resp PluginDetailResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Name != "echo" {
			t.Errorf("expected name echo, got %s", resp.Name)
		}
		if resp.Protocol != 2 {
			t.Errorf("expected protocol 2, got %d", resp.Protocol)
		}
		if len(resp.Commands) != 1 {
			t.Fatalf("expected 1 command, got %d", len(resp.Commands))
		}
		if resp.Commands[0].Name != "poll" {
			t.Errorf("expected command poll, got %s", resp.Commands[0].Name)
		}
		if resp.Commands[0].Description != "Poll command" {
			t.Errorf("expected description Poll command, got %s", resp.Commands[0].Description)
		}

		// Verify schema
		schemaBytes, _ := json.Marshal(resp.Commands[0].InputSchema)
		expectedBytes, _ := json.Marshal(inputSchema)
		if string(schemaBytes) != string(expectedBytes) {
			t.Errorf("expected schema %s, got %s", string(expectedBytes), string(schemaBytes))
		}
	})

	t.Run("compact schema expansion", func(t *testing.T) {
		reg := &mockRegistry{
			plugins: map[string]*plugin.Plugin{
				"compact": {
					Name: "compact",
					Commands: plugin.Commands{
						{
							Name: "test",
							InputSchema: map[string]any{
								"msg": "string",
								"val": "integer",
							},
						},
					},
				},
			},
		}
		server := newTestServer(&mockQueue{}, reg)

		req := httptest.NewRequest(http.MethodGet, "/plugin/compact", nil)
		req.Header.Set("Authorization", "Bearer test-key-123")
		rr := httptest.NewRecorder()

		server.setupRoutes().ServeHTTP(rr, req)

		var resp PluginDetailResponse
		json.NewDecoder(rr.Body).Decode(&resp)

		cmd := resp.Commands[0]
		schema, ok := cmd.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("expected map schema, got %T", cmd.InputSchema)
		}
		if schema["type"] != "object" {
			t.Errorf("expected type object, got %v", schema["type"])
		}
		props := schema["properties"].(map[string]any)
		if props["msg"].(map[string]any)["type"] != "string" {
			t.Errorf("expected msg type string, got %v", props["msg"])
		}
		if props["val"].(map[string]any)["type"] != "integer" {
			t.Errorf("expected val type integer, got %v", props["val"])
		}
	})

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/plugin/unknown", nil)
		req.Header.Set("Authorization", "Bearer test-key-123")
		rr := httptest.NewRecorder()

		server.setupRoutes().ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", rr.Code)
		}
	})
}

func TestHandleWellKnownPlugin_NoAuth(t *testing.T) {
	server := newTestServer(&mockQueue{}, &mockRegistry{})

	req := httptest.NewRequest(http.MethodGet, "/.well-known/ai-plugin.json", nil)
	rr := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var manifest map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&manifest); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if manifest["schema_version"] != "v1" {
		t.Fatalf("schema_version = %v, want v1", manifest["schema_version"])
	}
	if manifest["name_for_model"] != "ductile" {
		t.Fatalf("name_for_model = %v, want ductile", manifest["name_for_model"])
	}

	api, ok := manifest["api"].(map[string]any)
	if !ok {
		t.Fatalf("expected api object in manifest")
	}
	if api["type"] != "openapi" {
		t.Fatalf("api.type = %v, want openapi", api["type"])
	}
	if api["url"] != "/openapi.json" {
		t.Fatalf("api.url = %v, want /openapi.json", api["url"])
	}
}
