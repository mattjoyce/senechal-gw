package webhook

import (
	"testing"
)

func TestVerifyHMACSignature(t *testing.T) {
	secret := "test-secret-key"
	body := []byte(`{"event":"push","repository":"test"}`)

	// Compute expected signature
	expectedSig := computeExpectedSignature(body, secret)

	tests := []struct {
		name      string
		body      []byte
		signature string
		secret    string
		wantErr   bool
	}{
		{
			name:      "valid signature - plain hex",
			body:      body,
			signature: expectedSig,
			secret:    secret,
			wantErr:   false,
		},
		{
			name:      "valid signature - GitHub format",
			body:      body,
			signature: formatGitHubSignature(expectedSig),
			secret:    secret,
			wantErr:   false,
		},
		{
			name:      "invalid signature - wrong signature",
			body:      body,
			signature: "0000000000000000000000000000000000000000000000000000000000000000",
			secret:    secret,
			wantErr:   true,
		},
		{
			name:      "invalid signature - tampered body",
			body:      []byte(`{"event":"push","repository":"hacked"}`),
			signature: expectedSig,
			secret:    secret,
			wantErr:   true,
		},
		{
			name:      "invalid signature - wrong secret",
			body:      body,
			signature: expectedSig,
			secret:    "wrong-secret",
			wantErr:   true,
		},
		{
			name:      "invalid signature - empty signature",
			body:      body,
			signature: "",
			secret:    secret,
			wantErr:   true,
		},
		{
			name:      "invalid signature - empty secret",
			body:      body,
			signature: expectedSig,
			secret:    "",
			wantErr:   true,
		},
		{
			name:      "invalid signature - malformed hex",
			body:      body,
			signature: "not-valid-hex",
			secret:    secret,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyHMACSignature(tt.body, tt.signature, tt.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("verifyHMACSignature() error = %v, wantErr %v", err, tt.wantErr)
			}

			// All errors should be generic (no information leakage)
			if err != nil && err.Error() != "webhook verification failed" {
				t.Errorf("error should be generic, got: %v", err)
			}
		})
	}
}

func TestParseSignature(t *testing.T) {
	tests := []struct {
		name      string
		signature string
		want      string // hex representation of expected bytes
		wantErr   bool
	}{
		{
			name:      "GitHub format - sha256 prefix",
			signature: "sha256=3a8f7b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a",
			want:      "3a8f7b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a",
			wantErr:   false,
		},
		{
			name:      "plain hex",
			signature: "3a8f7b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a",
			want:      "3a8f7b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a",
			wantErr:   false,
		},
		{
			name:      "invalid hex",
			signature: "not-valid-hex",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSignature(tt.signature)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSignature() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				gotHex := ""
				for _, b := range got {
					gotHex += string("0123456789abcdef"[b>>4])
					gotHex += string("0123456789abcdef"[b&0xf])
				}
				if gotHex != tt.want {
					t.Errorf("parseSignature() = %v, want %v", gotHex, tt.want)
				}
			}
		})
	}
}

func TestComputeExpectedSignature(t *testing.T) {
	body := []byte("test payload")
	secret := "test-secret"

	sig := computeExpectedSignature(body, secret)

	// Should return lowercase hex string
	if len(sig) != 64 { // SHA256 = 32 bytes = 64 hex chars
		t.Errorf("signature length = %d, want 64", len(sig))
	}

	// Should be deterministic
	sig2 := computeExpectedSignature(body, secret)
	if sig != sig2 {
		t.Error("signature should be deterministic")
	}

	// Different body should produce different signature
	sig3 := computeExpectedSignature([]byte("different"), secret)
	if sig == sig3 {
		t.Error("different body should produce different signature")
	}
}
