package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadAndCompileDir discovers pipeline files in <configDir>/pipelines/*.yaml,
// parses all definitions, and compiles them into validated DAGs.
func LoadAndCompileDir(configDir string) (*Set, error) {
	pipelinesDir := filepath.Join(configDir, "pipelines")
	entries, err := os.ReadDir(pipelinesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &Set{Pipelines: map[string]*Pipeline{}}, nil
		}
		return nil, fmt.Errorf("read pipelines directory %q: %w", pipelinesDir, err)
	}

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

	var specs []PipelineSpec
	for _, filePath := range yamlFiles {
		fileSpec, err := LoadFile(filePath)
		if err != nil {
			return nil, err
		}
		specs = append(specs, fileSpec.Pipelines...)
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
