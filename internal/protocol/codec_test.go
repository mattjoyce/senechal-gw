package protocol

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestEncodeRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *Request
		wantErr bool
		checkFn func(t *testing.T, output string)
	}{
		{
			name: "valid poll request",
			req: &Request{
				Protocol:   1,
				JobID:      "test-job-123",
				Command:    "poll",
				Config:     map[string]any{"key": "value"},
				State:      map[string]any{"last_run": "2026-01-01"},
				DeadlineAt: time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			checkFn: func(t *testing.T, output string) {
				if !strings.Contains(output, `"protocol":1`) {
					t.Error("missing protocol field")
				}
				if !strings.Contains(output, `"job_id":"test-job-123"`) {
					t.Error("missing job_id field")
				}
				if !strings.Contains(output, `"command":"poll"`) {
					t.Error("missing command field")
				}
			},
		},
		{
			name: "unsupported protocol version",
			req: &Request{
				Protocol: 2,
				JobID:    "test",
				Command:  "poll",
			},
			wantErr: true,
		},
		{
			name: "handle request with event",
			req: &Request{
				Protocol: 1,
				JobID:    "test-job-456",
				Command:  "handle",
				Config:   map[string]any{},
				State:    map[string]any{},
				Event: &Event{
					Type:    "data_ready",
					Payload: map[string]any{"value": 42},
				},
				DeadlineAt: time.Now(),
			},
			wantErr: false,
			checkFn: func(t *testing.T, output string) {
				if !strings.Contains(output, `"command":"handle"`) {
					t.Error("missing command field")
				}
				if !strings.Contains(output, `"event"`) {
					t.Error("missing event field for handle command")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := EncodeRequest(&buf, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("EncodeRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFn != nil {
				tt.checkFn(t, buf.String())
			}
		})
	}
}

func TestDecodeResponse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		checkFn func(t *testing.T, resp *Response)
	}{
		{
			name:    "valid ok response",
			input:   `{"status":"ok","state_updates":{"last_run":"2026-02-08"}}`,
			wantErr: false,
			checkFn: func(t *testing.T, resp *Response) {
				if resp.Status != "ok" {
					t.Errorf("want status=ok, got %s", resp.Status)
				}
				if resp.StateUpdates["last_run"] != "2026-02-08" {
					t.Error("state_updates not parsed correctly")
				}
			},
		},
		{
			name:    "valid error response",
			input:   `{"status":"error","error":"something went wrong","retry":false}`,
			wantErr: false,
			checkFn: func(t *testing.T, resp *Response) {
				if resp.Status != "error" {
					t.Errorf("want status=error, got %s", resp.Status)
				}
				if resp.Error != "something went wrong" {
					t.Errorf("want error message, got %s", resp.Error)
				}
				if resp.ShouldRetry() {
					t.Error("want retry=false")
				}
			},
		},
		{
			name:    "retry defaults to true",
			input:   `{"status":"error","error":"temporary failure"}`,
			wantErr: false,
			checkFn: func(t *testing.T, resp *Response) {
				if !resp.ShouldRetry() {
					t.Error("want retry to default to true")
				}
			},
		},
		{
			name:    "response with events",
			input:   `{"status":"ok","events":[{"type":"data_ready","payload":{"value":42}}]}`,
			wantErr: false,
			checkFn: func(t *testing.T, resp *Response) {
				if len(resp.Events) != 1 {
					t.Fatalf("want 1 event, got %d", len(resp.Events))
				}
				if resp.Events[0].Type != "data_ready" {
					t.Error("event type not parsed")
				}
			},
		},
		{
			name:    "response with logs",
			input:   `{"status":"ok","logs":[{"level":"info","message":"test log"}]}`,
			wantErr: false,
			checkFn: func(t *testing.T, resp *Response) {
				if len(resp.Logs) != 1 {
					t.Fatalf("want 1 log, got %d", len(resp.Logs))
				}
				if resp.Logs[0].Level != "info" {
					t.Error("log level not parsed")
				}
			},
		},
		{
			name:    "missing status field",
			input:   `{"state_updates":{}}`,
			wantErr: true,
		},
		{
			name:    "invalid status value",
			input:   `{"status":"unknown"}`,
			wantErr: true,
		},
		{
			name:    "error status without message",
			input:   `{"status":"error"}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   `{not json}`,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			resp, err := DecodeResponse(reader)

			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFn != nil {
				tt.checkFn(t, resp)
			}
		})
	}
}

func TestDecodeResponseLenient(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		wantRawData bool
	}{
		{
			name:        "valid JSON response",
			input:       `{"status":"ok"}`,
			wantErr:     false,
			wantRawData: true,
		},
		{
			name:        "invalid JSON captures raw data",
			input:       `not json at all`,
			wantErr:     true,
			wantRawData: true,
		},
		{
			name:        "empty output",
			input:       ``,
			wantErr:     true,
			wantRawData: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			resp, rawData, err := DecodeResponseLenient(reader)

			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeResponseLenient() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantRawData && len(rawData) == 0 && tt.input != "" {
				t.Error("expected raw data to be captured")
			}

			if !tt.wantErr && resp == nil {
				t.Error("expected response to be parsed")
			}
		})
	}
}
