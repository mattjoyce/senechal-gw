package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

// TokenConfig is a bearer token with a set of scopes.
type TokenConfig struct {
	Token  string
	Scopes []string
}

type Principal struct {
	Token  string
	Scopes map[string]struct{}
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

func ExtractBearerToken(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", errors.New("missing Authorization header")
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return "", errors.New("invalid Authorization header format")
	}

	token := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	if token == "" {
		return "", errors.New("missing API key")
	}
	return token, nil
}

func constantTimeEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Authenticate matches a presented bearer token against configured tokens.
// If legacyAPIKey matches, it authenticates as admin with scope "*".
func Authenticate(presented string, legacyAPIKey string, tokens []TokenConfig) (Principal, bool) {
	if constantTimeEqual(presented, legacyAPIKey) {
		return Principal{
			Token:  presented,
			Scopes: map[string]struct{}{"*": {}},
		}, true
	}

	for _, t := range tokens {
		if constantTimeEqual(presented, t.Token) {
			return Principal{
				Token:  presented,
				Scopes: normalizeScopes(t.Scopes),
			}, true
		}
	}
	return Principal{}, false
}

func normalizeScopes(scopes []string) map[string]struct{} {
	out := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out[s] = struct{}{}
	}

	// Write implies read for well-known resources.
	if _, ok := out["plugin:rw"]; ok {
		out["plugin:ro"] = struct{}{}
	}
	if _, ok := out["jobs:rw"]; ok {
		out["jobs:ro"] = struct{}{}
	}
	if _, ok := out["events:rw"]; ok {
		out["events:ro"] = struct{}{}
	}
	return out
}

func HasAnyScope(p Principal, required ...string) bool {
	if len(required) == 0 {
		return true
	}
	if _, ok := p.Scopes["*"]; ok {
		return true
	}
	for _, s := range required {
		if _, ok := p.Scopes[s]; ok {
			return true
		}
	}
	return false
}
