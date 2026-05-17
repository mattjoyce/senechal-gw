package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseByteSize parses human-readable byte sizes like "1MB" or "1024".
func ParseByteSize(size string) (int64, error) {
	size = strings.TrimSpace(size)
	if size == "" {
		return 0, fmt.Errorf("size is empty")
	}
	// Preserve what the operator actually wrote for error reporting; the
	// parsing path uppercases and trims suffixes, and leaking that
	// transformed form breaks config-error -> config-line traceability.
	original := size

	upper := strings.ToUpper(size)
	multiplier := int64(1)

	switch {
	case strings.HasSuffix(upper, "KB"):
		multiplier = 1024
		size = strings.TrimSuffix(upper, "KB")
	case strings.HasSuffix(upper, "MB"):
		multiplier = 1024 * 1024
		size = strings.TrimSuffix(upper, "MB")
	case strings.HasSuffix(upper, "GB"):
		multiplier = 1024 * 1024 * 1024
		size = strings.TrimSuffix(upper, "GB")
	}

	value, err := strconv.ParseInt(strings.TrimSpace(size), 10, 64)
	if err != nil {
		// Do not wrap the strconv error: it embeds the code-transformed
		// substring, which is exactly the leak C-FRO-12 is about.
		return 0, fmt.Errorf("invalid size value %q: not a valid byte size", original)
	}
	if value <= 0 {
		return 0, fmt.Errorf("size %q must be positive", original)
	}
	if multiplier > 1 && value > (math.MaxInt64/multiplier) {
		return 0, fmt.Errorf("size %q too large", original)
	}

	result := value * multiplier
	if result < 0 {
		return 0, fmt.Errorf("size %q too large", original)
	}
	return result, nil
}
