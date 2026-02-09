package api

import (
	"crypto/subtle"
	"errors"
	"net/http"
)

// ValidateAPIKey returns true if providedKey matches configKey.
// If configKey is empty, callers should treat the API as effectively disabled.
func ValidateAPIKey(providedKey string, configKey string) bool {
	if configKey == "" || providedKey == "" {
		return false
	}
	if len(providedKey) != len(configKey) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(providedKey), []byte(configKey)) == 1
}

// ExtractAPIKey extracts an API key from an Authorization: Bearer <key> header.
func ExtractAPIKey(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", errors.New("missing Authorization header")
	}

	const prefix = "Bearer "
	if len(auth) < len(prefix) || auth[:len(prefix)] != prefix {
		return "", errors.New("invalid Authorization header format")
	}

	key := auth[len(prefix):]
	// Trim whitespace and check if empty
	if len(key) == 0 || len(key) > 0 && key[0] == ' ' {
		// Check if it's only whitespace
		trimmed := ""
		for _, c := range key {
			if c != ' ' && c != '\t' {
				trimmed += string(c)
			}
		}
		if trimmed == "" {
			return "", errors.New("missing API key")
		}
		return trimmed, nil
	}
	return key, nil
}

// authMiddleware validates the API key from Authorization header using Codex's functions
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey, err := ExtractAPIKey(r)
		if err != nil {
			s.writeError(w, http.StatusUnauthorized, err.Error())
			return
		}

		if !ValidateAPIKey(apiKey, s.config.APIKey) {
			s.writeError(w, http.StatusUnauthorized, "invalid API key")
			return
		}

		next.ServeHTTP(w, r)
	})
}
