package storage

import "testing"

func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"table", `"table"`},
		{`table"`, `"table"""`},
		{`"table"`, `"""table"""`},
		{"table; DROP TABLE users; --", `"table; DROP TABLE users; --"`},
	}

	for _, tc := range tests {
		got := quoteIdentifier(tc.input)
		if got != tc.expected {
			t.Errorf("quoteIdentifier(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
