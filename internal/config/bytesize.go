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
		return 0, fmt.Errorf("invalid size value: %w", err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("size must be positive")
	}
	if multiplier > 1 && value > (math.MaxInt64/multiplier) {
		return 0, fmt.Errorf("size too large")
	}

	result := value * multiplier
	if result < 0 {
		return 0, fmt.Errorf("size too large")
	}
	return result, nil
}
