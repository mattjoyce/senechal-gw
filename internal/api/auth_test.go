package api

import (
	"testing"

	"github.com/mattjoyce/ductile/internal/auth"
)

func TestAuthenticate_TokenScopes(t *testing.T) {
	t.Parallel()

	p, ok := auth.Authenticate("provided", []auth.TokenConfig{{Token: "provided", Scopes: []string{"plugin:rw"}}})
	if !ok {
		t.Fatalf("expected ok")
	}
	if !auth.HasAnyScope(p, "plugin:rw") {
		t.Fatalf("expected plugin:rw scope")
	}
	if !auth.HasAnyScope(p, "plugin:ro") {
		t.Fatalf("expected implied plugin:ro scope")
	}
}
