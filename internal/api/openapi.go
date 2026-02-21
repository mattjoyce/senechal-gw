package api

import (
	"fmt"
	"sort"

	"github.com/mattjoyce/ductile/internal/plugin"
)

// buildOpenAPIDoc returns an OpenAPI 3.1 document covering every command in the provided plugins.
func buildOpenAPIDoc(plugins map[string]*plugin.Plugin) map[string]any {
	paths := map[string]any{}

	names := make([]string, 0, len(plugins))
	for name := range plugins {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		for path, item := range buildPluginPaths(name, plugins[name]) {
			paths[path] = item
		}
	}

	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "Ductile Gateway",
			"version": "1.0",
		},
		"paths": paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"BearerAuth": map[string]any{
					"type":   "http",
					"scheme": "bearer",
				},
			},
		},
	}
}

// buildPluginPaths builds OpenAPI path items for a single plugin.
func buildPluginPaths(name string, p *plugin.Plugin) map[string]any {
	paths := map[string]any{}

	for _, cmd := range p.Commands {
		summary := cmd.Description
		if summary == "" {
			summary = fmt.Sprintf("%s: %s", name, cmd.Name)
		}

		operation := map[string]any{
			"operationId": fmt.Sprintf("%s__%s", name, cmd.Name),
			"summary":     summary,
			"tags":        []string{name},
			"responses": map[string]any{
				"202": map[string]any{"description": "Job queued"},
				"400": map[string]any{"description": "Bad request"},
				"403": map[string]any{"description": "Insufficient scope"},
			},
			"security": []any{map[string]any{"BearerAuth": []string{}}},
		}

		if schema := cmd.GetFullInputSchema(); schema != nil {
			operation["requestBody"] = map[string]any{
				"required": false,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": schema,
					},
				},
			}
		}

		paths[fmt.Sprintf("/plugin/%s/%s", name, cmd.Name)] = map[string]any{
			"post": operation,
		}
	}

	return paths
}
