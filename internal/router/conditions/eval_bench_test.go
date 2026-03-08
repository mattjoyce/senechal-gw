package conditions

import (
	"testing"
)

func BenchmarkEvalRegex(b *testing.B) {
	cond := Condition{
		Path:  "payload.code",
		Op:    OpRegex,
		Value: `(?i)^[a-z]{3}-\d{3}$`,
	}
	Validate(&cond) // Add this to compile the regex!

	scope := Scope{
		Payload: map[string]any{
			"code": "AbC-123",
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Eval(&cond, scope)
	}
}
