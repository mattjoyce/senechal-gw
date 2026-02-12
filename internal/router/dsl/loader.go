package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadAndCompileDir discovers pipeline files in <configDir>/pipelines/*.yaml
// and <configDir>/pipelines.yaml, parses all definitions, and compiles them into validated DAGs.
func LoadAndCompileDir(configDir string) (*Set, error) {
	var specs []PipelineSpec

	// 1. Try pipelines/ directory
	pipelinesDir := filepath.Join(configDir, "pipelines")
	entries, err := os.ReadDir(pipelinesDir)
	if err == nil {
		var yamlFiles []string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if filepath.Ext(entry.Name()) != ".yaml" {
				continue
			}
			yamlFiles = append(yamlFiles, filepath.Join(pipelinesDir, entry.Name()))
		}
		sort.Strings(yamlFiles)

		for _, filePath := range yamlFiles {
			fileSpec, err := LoadFile(filePath)
			if err != nil {
				return nil, err
			}
			specs = append(specs, fileSpec.Pipelines...)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read pipelines directory %q: %w", pipelinesDir, err)
	}

	// 2. Try pipelines.yaml file
	pipelinesFile := filepath.Join(configDir, "pipelines.yaml")
	if _, err := os.Stat(pipelinesFile); err == nil {
		fileSpec, err := LoadFile(pipelinesFile)
		if err != nil {
			return nil, err
		}
		specs = append(specs, fileSpec.Pipelines...)
	}

	if len(specs) == 0 {
		return &Set{Pipelines: map[string]*Pipeline{}}, nil
	}

	return CompileSpecs(specs)
}

// LoadFile parses one pipeline YAML file.
func LoadFile(path string) (*FileSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline file %q: %w", path, err)
	}

	var fileSpec FileSpec
	if err := yaml.Unmarshal(data, &fileSpec); err != nil {
		return nil, fmt.Errorf("parse pipeline file %q: %w", path, err)
	}

	for i, pipeline := range fileSpec.Pipelines {
		fileSpec.Pipelines[i].Name = strings.TrimSpace(pipeline.Name)
		fileSpec.Pipelines[i].On = strings.TrimSpace(pipeline.On)
	}
	return &fileSpec, nil
}
