package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// verifyHMACSignature verifies an HMAC-SHA256 signature against the request body.
//
// This function uses constant-time comparison (crypto/subtle) to prevent timing attacks.
// It supports multiple signature formats commonly used by webhook providers.
//
// Supported formats:
//   - "sha256=<hex>" (GitHub style)
//   - "<hex>" (plain hex)
//
// Returns nil if signature is valid, error otherwise.
// All errors are generic to prevent information leakage.
func verifyHMACSignature(body []byte, signature, secret string) error {
	if secret == "" {
		return fmt.Errorf("webhook verification failed")
	}

	if signature == "" {
		return fmt.Errorf("webhook verification failed")
	}

	// Compute HMAC-SHA256 of request body
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedMAC := mac.Sum(nil)

	// Parse signature from header (handle different formats)
	actualMAC, err := parseSignature(signature)
	if err != nil {
		// Generic error - don't leak format details
		return fmt.Errorf("webhook verification failed")
	}

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare(expectedMAC, actualMAC) != 1 {
		return fmt.Errorf("webhook verification failed")
	}

	return nil
}

// parseSignature extracts and decodes the HMAC signature from various formats.
//
// Supported formats:
//   - "sha256=3a8f..." (GitHub X-Hub-Signature-256)
//   - "3a8f..." (plain hex)
//
// Returns the raw bytes of the signature.
func parseSignature(signature string) ([]byte, error) {
	// Handle GitHub-style "sha256=<hex>" format
	if strings.HasPrefix(signature, "sha256=") {
		hexSig := strings.TrimPrefix(signature, "sha256=")
		return hex.DecodeString(hexSig)
	}

	// Handle plain hex format
	return hex.DecodeString(signature)
}

// computeExpectedSignature computes the HMAC-SHA256 signature for a body.
// Used for testing and validation. Returns hex-encoded signature.
func computeExpectedSignature(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// formatGitHubSignature formats a hex signature in GitHub's X-Hub-Signature-256 format.
func formatGitHubSignature(hexSig string) string {
	return "sha256=" + hexSig
}
