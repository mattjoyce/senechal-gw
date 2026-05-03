package relay

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

func sign(method, path, timestamp string, body []byte, secret string) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("relay signing failed")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.ToUpper(strings.TrimSpace(method))))
	mac.Write([]byte{'\n'})
	mac.Write([]byte(path))
	mac.Write([]byte{'\n'})
	mac.Write([]byte(strings.TrimSpace(timestamp)))
	mac.Write([]byte{'\n'})
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func verifySignature(method, path, timestamp string, body []byte, signature, secret string) error {
	expected, err := sign(method, path, timestamp, body, secret)
	if err != nil {
		return fmt.Errorf("relay verification failed")
	}
	actual, err := parseSignature(signature)
	if err != nil {
		return fmt.Errorf("relay verification failed")
	}
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return fmt.Errorf("relay verification failed")
	}
	if subtle.ConstantTimeCompare(expectedBytes, actual) != 1 {
		return fmt.Errorf("relay verification failed")
	}
	return nil
}

func parseSignature(signature string) ([]byte, error) {
	signature = strings.TrimSpace(signature)
	if after, ok := strings.CutPrefix(signature, "sha256="); ok {
		signature = after
	}
	return hex.DecodeString(signature)
}
