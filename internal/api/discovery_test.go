package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mattjoyce/ductile/internal/plugin"
)

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
