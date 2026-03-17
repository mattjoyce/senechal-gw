package webhook

import (
	"testing"
)

func TestParseMaxBodySize(t *testing.T) {
	tests := []struct {
		name    string
		size    string
		want    int64
		wantErr bool
	}{
		{
			name:    "empty string returns default",
			size:    "",
			want:    DefaultMaxBodySize,
			wantErr: false,
		},
		{
			name:    "numeric bytes",
			size:    "1024",
			want:    1024,
			wantErr: false,
		},
		{
			name:    "KB suffix",
			size:    "1KB",
			want:    1024,
			wantErr: false,
		},
		{
			name:    "kb suffix lowercase",
			size:    "2kb",
			want:    2048,
			wantErr: false,
		},
		{
			name:    "MB suffix",
			size:    "1MB",
			want:    1048576,
			wantErr: false,
		},
		{
			name:    "GB suffix",
			size:    "1GB",
			want:    1073741824,
			wantErr: false,
		},
		{
			name:    "whitespace handling",
			size:    "  2 MB  ",
			want:    2097152,
			wantErr: false,
		},
		{
			name:    "zero value error",
			size:    "0",
			wantErr: true,
		},
		{
			name:    "negative value error",
			size:    "-1",
			wantErr: true,
		},
		{
			name:    "invalid format error",
			size:    "abc",
			wantErr: true,
		},
		{
			name:    "floating point error",
			size:    "1.5MB",
			wantErr: true,
		},
		{
			name:    "unsupported unit error",
			size:    "1TB",
			wantErr: true,
		},
		{
			name:    "too large numeric value",
			size:    "9223372036854775808",
			wantErr: true,
		},
		{
			name:    "overflow GB case",
			size:    "17179869185GB",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMaxBodySize(tt.size)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMaxBodySize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseMaxBodySize() = %v, want %v", got, tt.want)
			}
		})
	}
}
