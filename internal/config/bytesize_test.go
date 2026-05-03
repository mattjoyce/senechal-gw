package config

import "testing"

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		name    string
		size    string
		want    int64
		wantErr bool
	}{
		{name: "bytes", size: "1024", want: 1024},
		{name: "kb", size: "2KB", want: 2048},
		{name: "mb", size: "1MB", want: 1024 * 1024},
		{name: "gb", size: "1GB", want: 1024 * 1024 * 1024},
		{name: "whitespace", size: " 3 MB ", want: 3 * 1024 * 1024},
		{name: "empty", size: "", wantErr: true},
		{name: "zero", size: "0", wantErr: true},
		{name: "invalid", size: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseByteSize(tt.size)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseByteSize() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("ParseByteSize() = %d, want %d", got, tt.want)
			}
		})
	}
}
