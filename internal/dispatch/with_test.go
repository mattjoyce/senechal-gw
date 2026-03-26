package dispatch

import (
	"strings"
	"testing"

	"github.com/mattjoyce/ductile/internal/router/conditions"
)

func TestApplyWithRemapUsesSnapshotScope(t *testing.T) {
	payload := map[string]any{
		"message": "old",
		"stdout":  "hello",
	}
	with := map[string]string{
		"message": "{payload.stdout}",
		"mirror":  "{payload.message}",
	}

	got, err := applyWithRemap(payload, with, nil)
	if err != nil {
		t.Fatalf("applyWithRemap() error = %v", err)
	}
	if got["message"] != "hello" {
		t.Fatalf("message = %#v, want %q", got["message"], "hello")
	}
	if got["mirror"] != "old" {
		t.Fatalf("mirror = %#v, want %q", got["mirror"], "old")
	}
	if payload["message"] != "old" {
		t.Fatalf("input payload mutated: %#v", payload)
	}
}

func TestEvalWithTemplate(t *testing.T) {
	scope := conditions.Scope{
		Payload: map[string]any{
			"count":   3,
			"message": "hello",
		},
		Context: map[string]any{
			"user": "matt",
		},
	}

	tests := []struct {
		name    string
		tmpl    string
		want    any
		wantErr string
	}{
		{
			name: "pure reference preserves type",
			tmpl: "{payload.count}",
			want: 3,
		},
		{
			name: "mixed template interpolates",
			tmpl: "hi {context.user}: {payload.message}",
			want: "hi matt: hello",
		},
		{
			name:    "missing path fails",
			tmpl:    "{payload.missing}",
			wantErr: "path not found",
		},
		{
			name:    "invalid root fails",
			tmpl:    "{state.flag}",
			wantErr: "unsupported path root",
		},
		{
			name:    "unclosed brace fails",
			tmpl:    "{payload.message",
			wantErr: "unclosed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evalWithTemplate(tt.tmpl, scope)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("evalWithTemplate() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("evalWithTemplate() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}
