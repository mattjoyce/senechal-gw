package conditions

// Eval evaluates a validated condition tree against the provided scope.
func Eval(cond *Condition, scope Scope) (bool, error) {
	switch {
	case len(cond.All) > 0:
		for i := range cond.All {
			ok, err := Eval(&cond.All[i], scope)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	case len(cond.Any) > 0:
		for i := range cond.Any {
			ok, err := Eval(&cond.Any[i], scope)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	case cond.Not != nil:
		ok, err := Eval(cond.Not, scope)
		if err != nil {
			return false, err
		}
		return !ok, nil
	default:
		return evalAtomic(cond, scope)
	}
}
