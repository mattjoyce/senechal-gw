package api

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
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
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", errors.New("missing or invalid Authorization header")
	}
	key := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if key == "" {
		return "", errors.New("missing or invalid Authorization header")
	}
	return key, nil
}
