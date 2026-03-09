package auth

import (
	"context"
	"net/http"
	"reflect"
	"testing"
)

func TestAuthenticate(t *testing.T) {
	tokens := []TokenConfig{
		{Token: "valid-token-1", Scopes: []string{"read", "write"}},
		{Token: "valid-token-2", Scopes: []string{"admin"}},
		{Token: "plugin-token", Scopes: []string{"plugin:rw"}},
	}

	tests := []struct {
		name          string
		presented     string
		tokens        []TokenConfig
		wantPrincipal Principal
		wantBool      bool
	}{
		{
			name:      "valid token 1",
			presented: "valid-token-1",
			tokens:    tokens,
			wantPrincipal: Principal{
				Token: "valid-token-1",
				Scopes: map[string]struct{}{
					"read":  {},
					"write": {},
				},
			},
			wantBool: true,
		},
		{
			name:      "valid token 2",
			presented: "valid-token-2",
			tokens:    tokens,
			wantPrincipal: Principal{
				Token: "valid-token-2",
				Scopes: map[string]struct{}{
					"admin": {},
				},
			},
			wantBool: true,
		},
		{
			name:      "invalid token",
			presented: "invalid-token",
			tokens:    tokens,
			wantPrincipal: Principal{},
			wantBool:      false,
		},
		{
			name:      "empty presented token",
			presented: "",
			tokens:    tokens,
			wantPrincipal: Principal{},
			wantBool:      false,
		},
		{
			name:      "empty configured tokens",
			presented: "valid-token-1",
			tokens:    []TokenConfig{},
			wantPrincipal: Principal{},
			wantBool:      false,
		},
		{
			name:      "plugin rw scope expands to ro",
			presented: "plugin-token",
			tokens:    tokens,
			wantPrincipal: Principal{
				Token: "plugin-token",
				Scopes: map[string]struct{}{
					"plugin:rw": {},
					"plugin:ro": {},
				},
			},
			wantBool: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrincipal, gotBool := Authenticate(tt.presented, tt.tokens)
			if gotBool != tt.wantBool {
				t.Errorf("Authenticate() gotBool = %v, want %v", gotBool, tt.wantBool)
			}
			if !reflect.DeepEqual(gotPrincipal, tt.wantPrincipal) {
				t.Errorf("Authenticate() gotPrincipal = %v, want %v", gotPrincipal, tt.wantPrincipal)
			}
		})
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name      string
		authValue string
		want      string
		wantErr   bool
	}{
		{
			name:      "valid bearer token",
			authValue: "Bearer my-secret-token",
			want:      "my-secret-token",
			wantErr:   false,
		},
		{
			name:      "missing header",
			authValue: "",
			want:      "",
			wantErr:   true,
		},
		{
			name:      "invalid format",
			authValue: "Basic dXNlcm5hbWU6cGFzc3dvcmQ=",
			want:      "",
			wantErr:   true,
		},
		{
			name:      "empty token after prefix",
			authValue: "Bearer ",
			want:      "",
			wantErr:   true,
		},
		{
			name:      "whitespace token",
			authValue: "Bearer    ",
			want:      "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/", nil)
			if tt.authValue != "" {
				req.Header.Set("Authorization", tt.authValue)
			}
			got, err := ExtractBearerToken(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractBearerToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractBearerToken() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeScopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		want   map[string]struct{}
	}{
		{
			name:   "empty",
			scopes: []string{},
			want:   map[string]struct{}{},
		},
		{
			name:   "basic scopes",
			scopes: []string{"read", "write"},
			want: map[string]struct{}{
				"read":  {},
				"write": {},
			},
		},
		{
			name:   "with whitespace",
			scopes: []string{"  read  ", "write ", ""},
			want: map[string]struct{}{
				"read":  {},
				"write": {},
			},
		},
		{
			name:   "plugin rw expansion",
			scopes: []string{"plugin:rw"},
			want: map[string]struct{}{
				"plugin:rw": {},
				"plugin:ro": {},
			},
		},
		{
			name:   "jobs rw expansion",
			scopes: []string{"jobs:rw"},
			want: map[string]struct{}{
				"jobs:rw": {},
				"jobs:ro": {},
			},
		},
		{
			name:   "events rw expansion",
			scopes: []string{"events:rw"},
			want: map[string]struct{}{
				"events:rw": {},
				"events:ro": {},
			},
		},
		{
			name:   "duplicate scopes",
			scopes: []string{"read", "read"},
			want: map[string]struct{}{
				"read": {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeScopes(tt.scopes); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("normalizeScopes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasAnyScope(t *testing.T) {
	tests := []struct {
		name     string
		scopes   map[string]struct{}
		required []string
		want     bool
	}{
		{
			name:     "no required scopes",
			scopes:   map[string]struct{}{"read": {}},
			required: []string{},
			want:     true,
		},
		{
			name:     "has required scope",
			scopes:   map[string]struct{}{"read": {}, "write": {}},
			required: []string{"write"},
			want:     true,
		},
		{
			name:     "missing required scope",
			scopes:   map[string]struct{}{"read": {}},
			required: []string{"write"},
			want:     false,
		},
		{
			name:     "wildcard scope",
			scopes:   map[string]struct{}{"*": {}},
			required: []string{"admin"},
			want:     true,
		},
		{
			name:     "has one of required scopes",
			scopes:   map[string]struct{}{"read": {}},
			required: []string{"write", "read"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Principal{Scopes: tt.scopes}
			if got := HasAnyScope(p, tt.required...); got != tt.want {
				t.Errorf("HasAnyScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrincipalContext(t *testing.T) {
	ctx := context.Background()
	p := Principal{
		Token:  "test-token",
		Scopes: map[string]struct{}{"test": {}},
	}

	// Test missing
	_, ok := PrincipalFromContext(ctx)
	if ok {
		t.Error("PrincipalFromContext() returned true for empty context")
	}

	// Test with principal
	ctxWithP := WithPrincipal(ctx, p)
	got, ok := PrincipalFromContext(ctxWithP)
	if !ok {
		t.Error("PrincipalFromContext() returned false for context with principal")
	}
	if !reflect.DeepEqual(got, p) {
		t.Errorf("PrincipalFromContext() = %v, want %v", got, p)
	}
}

func TestConstantTimeEqual(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{
			name: "equal",
			a:    "secret",
			b:    "secret",
			want: true,
		},
		{
			name: "different length",
			a:    "secret",
			b:    "secrett",
			want: false,
		},
		{
			name: "different content",
			a:    "secret",
			b:    "secred",
			want: false,
		},
		{
			name: "empty a",
			a:    "",
			b:    "secret",
			want: false,
		},
		{
			name: "empty b",
			a:    "secret",
			b:    "",
			want: false,
		},
		{
			name: "both empty",
			a:    "",
			b:    "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := constantTimeEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("constantTimeEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
