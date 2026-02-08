package protocol

import (
	"encoding/json"
	"fmt"
	"io"
)

// EncodeRequest serializes a Request to JSON and writes it to w.
// Returns an error if marshaling or writing fails.
func EncodeRequest(w io.Writer, req *Request) error {
	if req.Protocol != 1 {
		return fmt.Errorf("unsupported protocol version: %d", req.Protocol)
	}

	encoder := json.NewEncoder(w)
	if err := encoder.Encode(req); err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	return nil
}

// DecodeResponse reads and deserializes a Response from JSON in r.
// Returns an error if reading or unmarshaling fails, or if the response is invalid.
func DecodeResponse(r io.Reader) (*Response, error) {
	var resp Response

	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields() // Strict parsing

	if err := decoder.Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Validate required fields
	if resp.Status == "" {
		return nil, fmt.Errorf("response missing required field: status")
	}

	if resp.Status != "ok" && resp.Status != "error" {
		return nil, fmt.Errorf("invalid status value: %q (must be 'ok' or 'error')", resp.Status)
	}

	// If status is error, error message should be present
	if resp.Status == "error" && resp.Error == "" {
		return nil, fmt.Errorf("response has status=error but no error message")
	}

	return &resp, nil
}

// DecodeResponseLenient is like DecodeResponse but captures any JSON on stdout.
// Used when debugging protocol errors - returns raw bytes if strict decode fails.
func DecodeResponseLenient(r io.Reader) (*Response, []byte, error) {
	// Read all bytes first
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	if len(data) == 0 {
		return nil, data, fmt.Errorf("plugin produced no output on stdout")
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, data, fmt.Errorf("plugin output is not valid JSON: %w", err)
	}

	// Validate
	if resp.Status == "" {
		return nil, data, fmt.Errorf("response missing required field: status")
	}

	if resp.Status != "ok" && resp.Status != "error" {
		return nil, data, fmt.Errorf("invalid status value: %q", resp.Status)
	}

	if resp.Status == "error" && resp.Error == "" {
		return nil, data, fmt.Errorf("response has status=error but no error message")
	}

	return &resp, data, nil
}
