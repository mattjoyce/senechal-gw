package state

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// PipelineInstanceNamespace is the reserved durable baggage namespace for
	// control-plane pipeline execution identity.
	PipelineInstanceNamespace = "ductile"
	// PipelineInstanceIDField is the durable key storing the pipeline execution
	// instance ID inside accumulated context baggage.
	PipelineInstanceIDField = "pipeline_instance_id"
)

// WithPipelineInstanceID merges the given pipeline execution instance ID into a
// JSON object of context updates.
func WithPipelineInstanceID(updates json.RawMessage, instanceID string) (json.RawMessage, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil, fmt.Errorf("pipeline instance id is empty")
	}

	obj := map[string]any{}
	if len(updates) > 0 {
		if err := json.Unmarshal(updates, &obj); err != nil {
			return nil, fmt.Errorf("decode context updates: %w", err)
		}
	}

	ductile, err := nestedObject(obj, PipelineInstanceNamespace)
	if err != nil {
		return nil, err
	}
	if existing, exists := ductile[PipelineInstanceIDField]; exists && existing != instanceID {
		return nil, fmt.Errorf("context updates already contain %s.%s=%v", PipelineInstanceNamespace, PipelineInstanceIDField, existing)
	}
	ductile[PipelineInstanceIDField] = instanceID
	obj[PipelineInstanceNamespace] = ductile

	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal context updates: %w", err)
	}
	return raw, nil
}

// PipelineInstanceIDFromAccumulated extracts the durable pipeline execution
// instance ID from accumulated context baggage.
func PipelineInstanceIDFromAccumulated(accumulated json.RawMessage) string {
	if len(accumulated) == 0 {
		return ""
	}

	var obj map[string]any
	if err := json.Unmarshal(accumulated, &obj); err != nil {
		return ""
	}

	ductile, ok := obj[PipelineInstanceNamespace].(map[string]any)
	if !ok {
		return ""
	}
	instanceID, _ := ductile[PipelineInstanceIDField].(string)
	return strings.TrimSpace(instanceID)
}

func nestedObject(obj map[string]any, key string) (map[string]any, error) {
	value, exists := obj[key]
	if !exists || value == nil {
		return map[string]any{}, nil
	}
	nested, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("context updates key %q must be an object", key)
	}
	return nested, nil
}
