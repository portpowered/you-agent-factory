package factorysessionexecution

import "errors"

// ErrExecutionRequestIDConflict reports that requestId was reused with a different
// normalized execution tuple.
var ErrExecutionRequestIDConflict = errors.New("execution request id conflict")

// ErrControlRequestIDConflict reports that requestId was reused with a different
// normalized lifecycle-control tuple.
var ErrControlRequestIDConflict = errors.New("control request id conflict")

// ErrSessionNotFound reports that no durable session matched the requested id.
var ErrSessionNotFound = errors.New("factory session not found")

// ErrDispatchNotFound reports that no dispatch matched the requested id within
// the targeted durable session.
var ErrDispatchNotFound = errors.New("dispatch not found")

// ErrUnsupportedControl reports that the requested control is not supported by
// the active durable session runtime.
var ErrUnsupportedControl = errors.New("unsupported lifecycle control")

// ControlError carries a typed lifecycle-control outcome for invalid transitions
// and other actionable control failures surfaced by service implementations.
type ControlError struct {
	Operation LifecycleControlKind
	Outcome   LifecycleControlOutcome
	Message   string
}

func (e *ControlError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Outcome)
}

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
