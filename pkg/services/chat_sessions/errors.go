package chatsessions

import (
	"errors"
	"fmt"
)

// Sentinel value-validation categories. Callers use errors.Is against these
// sentinels (typically through a returned *ValidationError) instead of
// parsing error text.
var (
	// ErrRequiredValue reports a blank or zero value where the L1 V0 contract
	// requires a caller-supplied identity, reference, or timestamp.
	ErrRequiredValue = errors.New("chat sessions: required value is blank or zero")
	// ErrUnknownEnumValue reports an enum value outside its declared members.
	ErrUnknownEnumValue = errors.New("chat sessions: unknown enum value")
	// ErrInconsistentValue reports a value whose fields disagree with each
	// other under the L1 V0 model, such as a terminal sequence or terminal
	// time fact that does not match the value's declared state.
	ErrInconsistentValue = errors.New("chat sessions: structurally inconsistent value")
	// ErrUnsupportedControlAction reports a ControlAction that is declared in
	// the L1 vocabulary for a later lane (PAUSE, RESUME, TERMINATE) but is not
	// an executable action in this L1 V0 slice. It is distinct from
	// ErrUnknownEnumValue, which reports a value outside the declared
	// vocabulary entirely.
	ErrUnsupportedControlAction = errors.New("chat sessions: control action is not supported in L1")
)

// ValidationError reports one Chat Sessions value-validation failure. Value
// names the owning type, Field names the offending field (empty when the
// failure applies to the whole value, such as an enum receiver), and Err is
// one of the package sentinel errors so callers can use errors.Is/errors.As
// without parsing Error() text.
type ValidationError struct {
	Value string
	Field string
	Err   error
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("chat sessions: %s: %v", e.Value, e.Err)
	}
	return fmt.Sprintf("chat sessions: %s.%s: %v", e.Value, e.Field, e.Err)
}

// Unwrap exposes the underlying sentinel (or nested *ValidationError) so
// errors.Is/errors.As can classify the failure across a wrapped chain.
func (e *ValidationError) Unwrap() error {
	return e.Err
}

func newValidationError(value, field string, err error) *ValidationError {
	return &ValidationError{Value: value, Field: field, Err: err}
}
