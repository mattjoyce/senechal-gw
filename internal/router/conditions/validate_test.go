package conditions

import "testing"

func TestValidateRejectsInvalidConditions(t *testing.T) {
	tests := []struct {
		name string
		cond Condition
	}{
		{name: "unknown op", cond: Condition{Path: "payload.status", Op: Operator("wat"), Value: "x"}},
		{name: "missing path", cond: Condition{Op: OpEq, Value: "x"}},
		{name: "exists with value", cond: Condition{Path: "payload.status", Op: OpExists, Value: true}},
		{name: "in requires array", cond: Condition{Path: "payload.status", Op: OpIn, Value: "x"}},
		{name: "illegal root", cond: Condition{Path: "state.flag", Op: OpEq, Value: true}},
		{name: "contains requires string value", cond: Condition{Path: "payload.status", Op: OpContains, Value: true}},
		{name: "regex requires string value", cond: Condition{Path: "payload.status", Op: OpRegex, Value: 123}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(&tt.cond); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestValidateDepthAndCountLimits(t *testing.T) {
	deep := Condition{Not: &Condition{Not: &Condition{Not: &Condition{Path: "payload.status", Op: OpEq, Value: "x"}}}}
	if err := Validate(&deep); err == nil {
		t.Fatalf("expected depth limit error")
	}

	many := Condition{All: make([]Condition, 0, 21)}
	for i := 0; i < 21; i++ {
		many.All = append(many.All, Condition{Path: "payload.status", Op: OpEq, Value: "x"})
	}
	if err := Validate(&many); err == nil {
		t.Fatalf("expected predicate count error")
	}
}

func TestValidateAcceptsStringOperators(t *testing.T) {
	tests := []Condition{
		{Path: "payload.text", Op: OpContains, Value: "needle"},
		{Path: "payload.text", Op: OpStartsWith, Value: "http"},
		{Path: "payload.text", Op: OpEndsWith, Value: ".pdf"},
		{Path: "payload.text", Op: OpRegex, Value: `(?i)foo.+bar`},
	}

	for _, cond := range tests {
		if err := Validate(&cond); err != nil {
			t.Fatalf("Validate(%+v) error = %v", cond, err)
		}
	}
}
