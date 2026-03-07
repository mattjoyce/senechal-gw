package conditions

import "fmt"

const (
	MaxDepthDefault      = 3
	MaxPredicatesDefault = 20
)

// ValidationError reports a precise condition validation failure.
type ValidationError struct {
	Path    string
	Message string
}

// Error returns the formatted validation error message.
func (e *ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

func newValidationError(path, message string) error {
	return &ValidationError{Path: path, Message: message}
}
