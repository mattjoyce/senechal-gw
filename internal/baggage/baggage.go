package baggage

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mattjoyce/ductile/internal/router/conditions"
	"github.com/mattjoyce/ductile/internal/router/dsl"
)

// ApplyClaims evaluates a baggage spec against the current payload and context.
func ApplyClaims(payload map[string]any, spec *dsl.BaggageSpec, ctx map[string]any) (map[string]any, error) {
	if spec == nil || spec.Empty() {
		return map[string]any{}, nil
	}

	scope := conditions.Scope{
		Payload: payload,
		Context: ctx,
	}
	out := make(map[string]any)

	if spec.Bulk != nil {
		if strings.TrimSpace(spec.Bulk.Namespace) == "" {
			return nil, fmt.Errorf("bulk baggage namespace is required until plugin manifest defaults are available")
		}
		present, value, err := conditions.ResolvePath(scope, spec.Bulk.From)
		if err != nil {
			return nil, fmt.Errorf("resolve baggage from %q: %w", spec.Bulk.From, err)
		}
		if !present {
			return nil, fmt.Errorf("resolve baggage from %q: path not found", spec.Bulk.From)
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("baggage from %q must resolve to an object", spec.Bulk.From)
		}
		if err := setPath(out, spec.Bulk.Namespace, object); err != nil {
			return nil, err
		}
	}

	for _, path := range sortedMappingKeys(spec.Mappings) {
		expr := spec.Mappings[path]
		present, value, err := conditions.ResolvePath(scope, expr)
		if err != nil {
			return nil, fmt.Errorf("resolve baggage %s from %q: %w", path, expr, err)
		}
		if !present {
			return nil, fmt.Errorf("resolve baggage %s from %q: path not found", path, expr)
		}
		if err := setPath(out, path, value); err != nil {
			return nil, err
		}
	}

	return out, nil
}

func setPath(dst map[string]any, path string, value any) error {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 {
		return fmt.Errorf("baggage path is empty")
	}

	current := dst
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("baggage path %q contains empty segment", path)
		}

		if i == len(parts)-1 {
			cloned, err := cloneValue(value)
			if err != nil {
				return fmt.Errorf("clone baggage path %q: %w", path, err)
			}
			current[part] = cloned
			return nil
		}

		next, exists := current[part]
		if !exists {
			child := make(map[string]any)
			current[part] = child
			current = child
			continue
		}

		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("baggage path %q conflicts with scalar prefix %q", path, strings.Join(parts[:i+1], "."))
		}
		current = child
	}
	return nil
}

func cloneValue(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedMappingKeys(mappings map[string]string) []string {
	keys := make([]string, 0, len(mappings))
	for key := range mappings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
