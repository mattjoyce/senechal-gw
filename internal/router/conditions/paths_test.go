package conditions

import "testing"

func TestResolvePath(t *testing.T) {
	scope := Scope{
		Payload: map[string]any{"status": "error", "nested": map[string]any{"count": 2}},
		Context: map[string]any{"origin_user": "matt"},
		Config:  map[string]any{"enabled": true},
	}

	tests := []struct {
		name    string
		path    string
		present bool
		value   any
		wantErr bool
	}{
		{name: "payload nested", path: "payload.nested.count", present: true, value: 2},
		{name: "context value", path: "context.origin_user", present: true, value: "matt"},
		{name: "config value", path: "config.enabled", present: true, value: true},
		{name: "missing key", path: "payload.missing", present: false, value: nil},
		{name: "illegal root", path: "state.flag", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			present, value, err := ResolvePath(scope, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolvePath error = %v", err)
			}
			if present != tt.present {
				t.Fatalf("present = %v, want %v", present, tt.present)
			}
			if value != tt.value {
				t.Fatalf("value = %#v, want %#v", value, tt.value)
			}
		})
	}
}
