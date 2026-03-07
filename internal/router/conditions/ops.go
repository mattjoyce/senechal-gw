package conditions

import (
	"fmt"
	"reflect"
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
		switch cond.Op {
		case OpGT:
			return left > right, nil
		case OpGTE:
			return left >= right, nil
		case OpLT:
			return left < right, nil
		case OpLTE:
			return left <= right, nil
		}
	}

	return false, fmt.Errorf("unsupported operator %q", cond.Op)
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
