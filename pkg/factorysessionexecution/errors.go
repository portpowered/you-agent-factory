package factorysessionexecution

import "errors"

// ErrExecutionRequestIDConflict reports that requestId was reused with a different
// normalized execution tuple.
var ErrExecutionRequestIDConflict = errors.New("execution request id conflict")

// ValidationError reports a stable client-side normalization failure.
type ValidationError struct {
	Message string
	Field   string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// NewValidationError constructs one field-scoped validation error.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}
