package dispatch

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mattjoyce/ductile/internal/router/conditions"
)

// applyWithRemap evaluates the step's with: map against the merged payload+context scope
// and adds/overrides keys in the event payload.
func applyWithRemap(payload map[string]any, with map[string]string, ctx map[string]any) (map[string]any, error) {
	if payload == nil {
		payload = make(map[string]any)
	}
	basePayload := clonePayloadMap(payload)
	remappedPayload := clonePayloadMap(payload)
	scope := conditions.Scope{
		Payload: basePayload,
		Context: ctx,
	}
	keys := sortedWithKeys(with)
	for _, key := range keys {
		value, err := evalWithTemplate(with[key], scope)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		remappedPayload[key] = value
	}
	return remappedPayload, nil
}

// evalWithTemplate evaluates a template string against a scope.
// If the entire string is "{path}", the resolved value is returned with its original type.
// Otherwise, each {path} occurrence is replaced with its string representation.
func evalWithTemplate(tmpl string, scope conditions.Scope) (any, error) {
	segments, err := parseWithTemplate(tmpl)
	if err != nil {
		return nil, err
	}

	if len(segments) == 1 && segments[0].path != "" {
		return resolveWithPath(segments[0].path, scope)
	}

	var out strings.Builder
	for _, segment := range segments {
		if segment.path == "" {
			out.WriteString(segment.literal)
			continue
		}
		value, err := resolveWithPath(segment.path, scope)
		if err != nil {
			return nil, err
		}
		if value != nil {
			_, _ = fmt.Fprintf(&out, "%v", value)
		}
	}
	return out.String(), nil
}

type withSegment struct {
	literal string
	path    string
}

func parseWithTemplate(tmpl string) ([]withSegment, error) {
	var segments []withSegment
	for len(tmpl) > 0 {
		start := strings.IndexByte(tmpl, '{')
		end := strings.IndexByte(tmpl, '}')
		if end >= 0 && (start == -1 || end < start) {
			return nil, fmt.Errorf("unexpected } in template %q", tmpl)
		}
		if start == -1 {
			segments = append(segments, withSegment{literal: tmpl})
			break
		}
		if start > 0 {
			segments = append(segments, withSegment{literal: tmpl[:start]})
		}
		end = strings.IndexByte(tmpl[start:], '}')
		if end == -1 {
			return nil, fmt.Errorf("unclosed { in template %q", tmpl)
		}
		end += start
		path := strings.TrimSpace(tmpl[start+1 : end])
		if path == "" {
			return nil, fmt.Errorf("empty template path in %q", tmpl)
		}
		if strings.Contains(path, "{") {
			return nil, fmt.Errorf("nested { in template %q", tmpl)
		}
		segments = append(segments, withSegment{path: path})
		tmpl = tmpl[end+1:]
	}
	if len(segments) == 0 {
		return []withSegment{{literal: ""}}, nil
	}
	return segments, nil
}

func resolveWithPath(path string, scope conditions.Scope) (any, error) {
	present, value, err := conditions.ResolvePath(scope, path)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", path, err)
	}
	if !present {
		return nil, fmt.Errorf("resolve %q: path not found", path)
	}
	return value, nil
}

func clonePayloadMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func sortedWithKeys(with map[string]string) []string {
	keys := make([]string, 0, len(with))
	for key := range with {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
