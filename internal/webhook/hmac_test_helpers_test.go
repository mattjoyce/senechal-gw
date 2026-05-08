package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func computeExpectedSignature(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func formatGitHubSignature(hexSig string) string {
	return "sha256=" + hexSig
}
