package conditions

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

func evalAtomic(cond Condition, scope Scope) (bool, error) {
	present, actual, err := ResolvePath(scope, cond.Path)
	if err != nil {
		return false, err
	}

	switch cond.Op {
	case OpExists:
		return present, nil
	case OpEq:
		return reflect.DeepEqual(actual, cond.Value), nil
	case OpNeq:
		return !reflect.DeepEqual(actual, cond.Value), nil
	case OpIn:
		values, ok := cond.Value.([]any)
		if !ok {
			return false, fmt.Errorf("operator %q requires array value", cond.Op)
		}
		for _, candidate := range values {
			if reflect.DeepEqual(actual, candidate) {
				return true, nil
			}
		}
		return false, nil
	case OpGT, OpGTE, OpLT, OpLTE:
		left, ok := asNumber(actual)
		if !ok {
			return false, fmt.Errorf("operator %q requires numeric path value", cond.Op)
		}
		right, ok := asNumber(cond.Value)
		if !ok {
			return false, fmt.Errorf("operator %q requires numeric comparison value", cond.Op)
		}
		return compareNumbers(cond.Op, left, right), nil
	case OpContains, OpStartsWith, OpEndsWith:
		left, right, err := stringOperands(cond, actual)
		if err != nil {
			return false, err
		}
		return compareStrings(cond.Op, left, right), nil
	case OpRegex:
		left, right, err := stringOperands(cond, actual)
		if err != nil {
			return false, err
		}
		re, err := regexp.Compile(right)
		if err != nil {
			return false, fmt.Errorf("operator %q received invalid regex: %w", cond.Op, err)
		}
		return re.MatchString(left), nil
	}

	return false, fmt.Errorf("unsupported operator %q", cond.Op)
}

func compareNumbers(op Operator, left, right float64) bool {
	switch op {
	case OpGT:
		return left > right
	case OpGTE:
		return left >= right
	case OpLT:
		return left < right
	case OpLTE:
		return left <= right
	default:
		return false
	}
}

func stringOperands(cond Condition, actual any) (string, string, error) {
	left, ok := actual.(string)
	if !ok {
		return "", "", fmt.Errorf("operator %q requires string path value", cond.Op)
	}
	right, ok := cond.Value.(string)
	if !ok {
		return "", "", fmt.Errorf("operator %q requires string comparison value", cond.Op)
	}
	return left, right, nil
}

func compareStrings(op Operator, left, right string) bool {
	leftFolded := strings.ToLower(left)
	rightFolded := strings.ToLower(right)

	switch op {
	case OpContains:
		return strings.Contains(leftFolded, rightFolded)
	case OpStartsWith:
		return strings.HasPrefix(leftFolded, rightFolded)
	case OpEndsWith:
		return strings.HasSuffix(leftFolded, rightFolded)
	default:
		return false
	}
}

func asNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}
