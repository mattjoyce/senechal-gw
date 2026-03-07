package conditions

import (
	"strings"
	"testing"
)

func TestEvalAtomicOperators(t *testing.T) {
	scope := Scope{Payload: map[string]any{"status": "Error: YouTube URL", "count": 3, "kind": "video.mp4", "code": "AbC-123"}}

	tests := []struct {
		name string
		cond Condition
		want bool
	}{
		{name: "exists true", cond: Condition{Path: "payload.status", Op: OpExists}, want: true},
		{name: "exists false", cond: Condition{Path: "payload.missing", Op: OpExists}, want: false},
		{name: "eq true", cond: Condition{Path: "payload.kind", Op: OpEq, Value: "video.mp4"}, want: true},
		{name: "neq true", cond: Condition{Path: "payload.status", Op: OpNeq, Value: "ok"}, want: true},
		{name: "in true", cond: Condition{Path: "payload.kind", Op: OpIn, Value: []any{"audio", "video.mp4"}}, want: true},
		{name: "gt true", cond: Condition{Path: "payload.count", Op: OpGT, Value: 2}, want: true},
		{name: "gte true", cond: Condition{Path: "payload.count", Op: OpGTE, Value: 3}, want: true},
		{name: "lt true", cond: Condition{Path: "payload.count", Op: OpLT, Value: 4}, want: true},
		{name: "lte true", cond: Condition{Path: "payload.count", Op: OpLTE, Value: 3}, want: true},
		{name: "contains is case insensitive", cond: Condition{Path: "payload.status", Op: OpContains, Value: "youtube"}, want: true},
		{name: "startswith is case insensitive", cond: Condition{Path: "payload.status", Op: OpStartsWith, Value: "error:"}, want: true},
		{name: "endswith is case insensitive", cond: Condition{Path: "payload.kind", Op: OpEndsWith, Value: ".MP4"}, want: true},
		{name: "regex matches full string", cond: Condition{Path: "payload.code", Op: OpRegex, Value: `(?i)^[a-z]{3}-\d{3}$`}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Eval(tt.cond, scope)
			if err != nil {
				t.Fatalf("Eval error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Eval = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvalCompositeConditions(t *testing.T) {
	scope := Scope{Payload: map[string]any{"kind": "video", "duration_sec": 45}}

	allCond := Condition{All: []Condition{{Path: "payload.kind", Op: OpEq, Value: "video"}, {Path: "payload.duration_sec", Op: OpGTE, Value: 30}}}
	ok, err := Eval(allCond, scope)
	if err != nil || !ok {
		t.Fatalf("all condition = %v, err=%v, want true nil", ok, err)
	}

	anyCond := Condition{Any: []Condition{{Path: "payload.kind", Op: OpEq, Value: "audio"}, {Path: "payload.duration_sec", Op: OpGTE, Value: 30}}}
	ok, err = Eval(anyCond, scope)
	if err != nil || !ok {
		t.Fatalf("any condition = %v, err=%v, want true nil", ok, err)
	}

	notCond := Condition{Not: &Condition{Path: "payload.kind", Op: OpEq, Value: "audio"}}
	ok, err = Eval(notCond, scope)
	if err != nil || !ok {
		t.Fatalf("not condition = %v, err=%v, want true nil", ok, err)
	}
}

func TestEvalStrictTypeMismatch(t *testing.T) {
	_, err := Eval(Condition{Path: "payload.count", Op: OpGT, Value: 1}, Scope{Payload: map[string]any{"count": "3"}})
	if err == nil {
		t.Fatalf("expected type mismatch error")
	}
}

func TestEvalStringOperatorTypeMismatch(t *testing.T) {
	_, err := Eval(Condition{Path: "payload.count", Op: OpContains, Value: "3"}, Scope{Payload: map[string]any{"count": 3}})
	if err == nil || !strings.Contains(err.Error(), "requires string path value") {
		t.Fatalf("err = %v, want string path value error", err)
	}
}

func TestEvalRegexRejectsInvalidPattern(t *testing.T) {
	_, err := Eval(Condition{Path: "payload.code", Op: OpRegex, Value: "("}, Scope{Payload: map[string]any{"code": "abc"}})
	if err == nil || !strings.Contains(err.Error(), "invalid regex") {
		t.Fatalf("err = %v, want invalid regex error", err)
	}
}
