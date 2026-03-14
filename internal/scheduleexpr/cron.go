package scheduleexpr

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronExpression represents a parsed 5-field cron schedule in UTC.
type CronExpression struct {
	minute cronField
	hour   cronField
	dom    cronField
	month  cronField
	dow    cronField
}

type cronField struct {
	allowed []bool
	any     bool
	min     int
	max     int
}

func (f cronField) matches(v int) bool {
	if v < f.min || v > f.max {
		return false
	}
	if v >= len(f.allowed) {
		return false
	}
	return f.allowed[v]
}

// ParseCron parses standard 5-field cron syntax: min hour dom month dow.
func ParseCron(expr string) (CronExpression, error) {
	trimmed := strings.TrimSpace(expr)
	parts := strings.Fields(trimmed)
	if len(parts) != 5 {
		return CronExpression{}, fmt.Errorf("invalid cron expression %q: expected 5 fields", expr)
	}

	minute, err := parseCronField(parts[0], 0, 59, nil, false)
	if err != nil {
		return CronExpression{}, fmt.Errorf("invalid cron minute field %q: %w", parts[0], err)
	}
	hour, err := parseCronField(parts[1], 0, 23, nil, false)
	if err != nil {
		return CronExpression{}, fmt.Errorf("invalid cron hour field %q: %w", parts[1], err)
	}
	dom, err := parseCronField(parts[2], 1, 31, nil, false)
	if err != nil {
		return CronExpression{}, fmt.Errorf("invalid cron day-of-month field %q: %w", parts[2], err)
	}
	month, err := parseCronField(parts[3], 1, 12, monthNames(), false)
	if err != nil {
		return CronExpression{}, fmt.Errorf("invalid cron month field %q: %w", parts[3], err)
	}
	dow, err := parseCronField(parts[4], 0, 7, weekdayNames(), true)
	if err != nil {
		return CronExpression{}, fmt.Errorf("invalid cron day-of-week field %q: %w", parts[4], err)
	}

	return CronExpression{minute: minute, hour: hour, dom: dom, month: month, dow: dow}, nil
}

// Matches reports whether t matches the cron expression (evaluated at minute granularity).
// The caller controls timezone by passing a time with the desired location.
func (c CronExpression) Matches(t time.Time) bool {
	if !c.minute.matches(int(t.Minute())) || !c.hour.matches(t.Hour()) || !c.month.matches(int(t.Month())) {
		return false
	}

	domMatch := c.dom.matches(t.Day())
	dowMatch := c.dow.matches(int(t.Weekday()))

	if c.dom.any && c.dow.any {
		return true
	}
	if c.dom.any {
		return dowMatch
	}
	if c.dow.any {
		return domMatch
	}
	// Standard cron behavior: dom and dow restricted => either can match.
	return domMatch || dowMatch
}

// NextAfter returns the next matching time strictly after from.
// The caller controls timezone by passing a time with the desired location.
func (c CronExpression) NextAfter(from time.Time) (time.Time, error) {
	candidate := from.Truncate(time.Minute).Add(time.Minute)
	deadline := candidate.AddDate(5, 0, 0)
	for !candidate.After(deadline) {
		if c.Matches(candidate) {
			return candidate, nil
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no matching time found within 5 years")
}

func parseCronField(raw string, min, max int, names map[string]int, dow bool) (cronField, error) {
	field := cronField{allowed: make([]bool, max+1), min: min, max: max}
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return field, fmt.Errorf("field is empty")
	}
	if raw == "*" {
		field.any = true
		for i := min; i <= max; i++ {
			field.allowed[i] = true
		}
		if dow {
			field.allowed[0] = true
		}
		return field, nil
	}

	for _, seg := range strings.Split(raw, ",") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return field, fmt.Errorf("empty list segment")
		}
		if err := applyCronSegment(field.allowed, seg, min, max, names, dow); err != nil {
			return field, err
		}
	}

	for i := min; i <= max; i++ {
		if field.allowed[i] {
			return field, nil
		}
	}
	return field, fmt.Errorf("no values selected")
}

func applyCronSegment(allowed []bool, seg string, min, max int, names map[string]int, dow bool) error {
	base := seg
	step := 1
	if strings.Contains(seg, "/") {
		parts := strings.Split(seg, "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid step segment %q", seg)
		}
		base = parts[0]
		parsedStep, err := strconv.Atoi(parts[1])
		if err != nil || parsedStep <= 0 {
			return fmt.Errorf("invalid step %q", parts[1])
		}
		step = parsedStep
	}

	if base == "*" {
		for i := min; i <= max; i += step {
			allowed[i] = true
		}
		if dow && allowed[7] {
			allowed[0] = true
		}
		return nil
	}

	start, end, err := parseCronRange(base, names, min, max, dow)
	if err != nil {
		return err
	}
	if start > end {
		return fmt.Errorf("descending range %q", base)
	}
	for i := start; i <= end; i += step {
		allowed[i] = true
	}
	if dow && allowed[7] {
		allowed[0] = true
	}
	return nil
}

func parseCronRange(raw string, names map[string]int, min, max int, dow bool) (int, int, error) {
	if strings.Contains(raw, "-") {
		parts := strings.Split(raw, "-")
		if len(parts) != 2 {
			return 0, 0, fmt.Errorf("invalid range %q", raw)
		}
		start, err := parseCronValue(parts[0], names, min, max, dow)
		if err != nil {
			return 0, 0, err
		}
		end, err := parseCronValue(parts[1], names, min, max, dow)
		if err != nil {
			return 0, 0, err
		}
		return start, end, nil
	}
	val, err := parseCronValue(raw, names, min, max, dow)
	if err != nil {
		return 0, 0, err
	}
	return val, val, nil
}

func parseCronValue(raw string, names map[string]int, min, max int, dow bool) (int, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}
	if names != nil {
		if v, ok := names[s]; ok {
			return v, nil
		}
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q", raw)
	}
	if dow && v == 7 {
		return 0, nil
	}
	if v < min || v > max {
		return 0, fmt.Errorf("value %d out of range [%d,%d]", v, min, max)
	}
	return v, nil
}

func monthNames() map[string]int {
	return map[string]int{
		"jan": 1,
		"feb": 2,
		"mar": 3,
		"apr": 4,
		"may": 5,
		"jun": 6,
		"jul": 7,
		"aug": 8,
		"sep": 9,
		"oct": 10,
		"nov": 11,
		"dec": 12,
	}
}

func weekdayNames() map[string]int {
	return map[string]int{
		"sun": 0,
		"mon": 1,
		"tue": 2,
		"wed": 3,
		"thu": 4,
		"fri": 5,
		"sat": 6,
	}
}
