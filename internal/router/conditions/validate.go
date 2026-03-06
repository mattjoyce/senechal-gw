package conditions

import (
	"fmt"
	"strings"
)

// Validate checks structural correctness and semantic limits for one condition tree.
func Validate(cond Condition) error {
	count := 0
	return validate(cond, "if", 1, &count)
}

func validate(cond Condition, at string, depth int, count *int) error {
	if depth > MaxDepthDefault {
		return newValidationError(at, fmt.Sprintf("maximum nesting depth exceeded (%d)", MaxDepthDefault))
	}
	*count++
	if *count > MaxPredicatesDefault {
		return newValidationError(at, fmt.Sprintf("maximum predicate count exceeded (%d)", MaxPredicatesDefault))
	}

	modes := 0
	if strings.TrimSpace(cond.Path) != "" || strings.TrimSpace(string(cond.Op)) != "" || cond.Value != nil {
		modes++
	}
	if len(cond.All) > 0 {
		modes++
	}
	if len(cond.Any) > 0 {
		modes++
	}
	if cond.Not != nil {
		modes++
	}
	if modes != 1 {
		return newValidationError(at, "must define exactly one of atomic predicate, all, any, or not")
	}

	if len(cond.All) > 0 {
		for i, child := range cond.All {
			if err := validate(child, fmt.Sprintf("%s.all[%d]", at, i), depth+1, count); err != nil {
				return err
			}
		}
		return nil
	}
	if len(cond.Any) > 0 {
		for i, child := range cond.Any {
			if err := validate(child, fmt.Sprintf("%s.any[%d]", at, i), depth+1, count); err != nil {
				return err
			}
		}
		return nil
	}
	if cond.Not != nil {
		return validate(*cond.Not, at+".not", depth+1, count)
	}

	if strings.TrimSpace(cond.Path) == "" {
		return newValidationError(at, "path is required")
	}
	if err := validatePath(cond.Path, at+".path"); err != nil {
		return err
	}
	if strings.TrimSpace(string(cond.Op)) == "" {
		return newValidationError(at, "op is required")
	}
	if !isSupportedOperator(cond.Op) {
		return newValidationError(at+".op", fmt.Sprintf("unsupported operator %q", cond.Op))
	}
	if cond.Op == OpExists {
		if cond.Value != nil {
			return newValidationError(at+".value", "value is not allowed for exists")
		}
		return nil
	}
	if cond.Value == nil {
		return newValidationError(at+".value", fmt.Sprintf("value is required for operator %q", cond.Op))
	}
	if cond.Op == OpIn {
		if _, ok := cond.Value.([]any); !ok {
			return newValidationError(at+".value", "value must be an array for in")
		}
	}
	return nil
}

func validatePath(path string, at string) error {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) < 2 {
		return newValidationError(at, "path must include one of payload.*, context.*, or config.*")
	}
	switch parts[0] {
	case "payload", "context", "config":
	default:
		return newValidationError(at, fmt.Sprintf("unsupported root %q", parts[0]))
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return newValidationError(at, "path contains empty segment")
		}
	}
	return nil
}

func isSupportedOperator(op Operator) bool {
	switch op {
	case OpExists, OpEq, OpNeq, OpIn, OpGT, OpGTE, OpLT, OpLTE:
		return true
	default:
		return false
	}
}
