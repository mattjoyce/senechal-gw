package api

import (
	"net/http"

	"github.com/mattjoyce/ductile/internal/auth"
)

// ExtractAPIKey extracts an API key from an Authorization: Bearer <key> header.
// Deprecated: use internal/auth.ExtractBearerToken.
func ExtractAPIKey(r *http.Request) (string, error) {
	return auth.ExtractBearerToken(r)
}

// authMiddleware authenticates the bearer token and attaches a Principal to context.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.ExtractBearerToken(r)
		if err != nil {
			s.writeError(w, http.StatusUnauthorized, err.Error())
			return
		}

		principal, ok := auth.Authenticate(token, s.config.APIKey, s.config.Tokens)
		if !ok {
			s.writeError(w, http.StatusUnauthorized, "invalid API key")
			return
		}

		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
}

// requireScopes enforces that the current Principal has at least one of the required scopes.
// Responds with 403 Forbidden on insufficient scope.
func (s *Server) requireScopes(required ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := auth.PrincipalFromContext(r.Context())
			if !ok {
				s.writeError(w, http.StatusUnauthorized, "missing Authorization context")
				return
			}
			if !auth.HasAnyScope(p, required...) {
				s.writeError(w, http.StatusForbidden, "insufficient scope")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
