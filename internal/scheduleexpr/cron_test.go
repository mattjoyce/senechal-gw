package scheduleexpr

import (
	"testing"
	"time"
)

func TestParseCronValid(t *testing.T) {
	tests := []string{
		"*/15 * * * *",
		"0 9 * * 1-5",
		"30 6 1 * *",
		"0 0 * jan mon",
	}
	for _, tc := range tests {
		if _, err := ParseCron(tc); err != nil {
			t.Fatalf("ParseCron(%q) err=%v", tc, err)
		}
	}
}

func TestParseCronInvalid(t *testing.T) {
	tests := []string{
		"",
		"* * * *",
		"* * * * * *",
		"61 * * * *",
		"* 24 * * *",
		"* * 0 * *",
		"* * * 13 *",
		"* * * * 8",
		"*/0 * * * *",
	}
	for _, tc := range tests {
		if _, err := ParseCron(tc); err == nil {
			t.Fatalf("ParseCron(%q) expected error", tc)
		}
	}
}

func TestCronMatches(t *testing.T) {
	expr, err := ParseCron("0 9 * * 1-5")
	if err != nil {
		t.Fatalf("ParseCron error: %v", err)
	}

	monday := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	if !expr.Matches(monday) {
		t.Fatalf("expected monday to match")
	}

	sunday := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	if expr.Matches(sunday) {
		t.Fatalf("expected sunday not to match")
	}
}

func TestCronNextAfter(t *testing.T) {
	expr, err := ParseCron("*/15 * * * *")
	if err != nil {
		t.Fatalf("ParseCron error: %v", err)
	}

	from := time.Date(2026, 3, 1, 10, 7, 31, 0, time.UTC)
	next, err := expr.NextAfter(from)
	if err != nil {
		t.Fatalf("NextAfter error: %v", err)
	}
	want := time.Date(2026, 3, 1, 10, 15, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next=%v want=%v", next, want)
	}
}
