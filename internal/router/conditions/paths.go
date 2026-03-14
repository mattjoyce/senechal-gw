package conditions

import (
	"fmt"
	"strings"
)

// ResolvePath resolves a dotted path from the supported scope roots.
func ResolvePath(scope Scope, path string) (bool, any, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false, nil, fmt.Errorf("path is empty")
	}

	parts := strings.Split(trimmed, ".")
	if len(parts) == 0 {
		return false, nil, fmt.Errorf("path is empty")
	}

	var current any
	switch parts[0] {
	case "payload":
		current = scope.Payload
	case "context":
		current = scope.Context
	case "config":
		current = scope.Config
	default:
		return false, nil, fmt.Errorf("unsupported path root %q", parts[0])
	}

	if len(parts) == 1 {
		return current != nil, current, nil
	}

	for _, segment := range parts[1:] {
		if strings.TrimSpace(segment) == "" {
			return false, nil, fmt.Errorf("path contains empty segment")
		}
		obj, ok := current.(map[string]any)
		if !ok {
			return false, nil, nil
		}
		next, exists := obj[segment]
		if !exists {
			return false, nil, nil
		}
		current = next
	}

	return true, current, nil
}
