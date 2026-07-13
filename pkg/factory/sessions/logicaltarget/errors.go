package logicaltarget

import (
	"errors"
	"fmt"
)

const (
	ReasonRequired        = "required"
	ReasonInvalidTarget   = "invalid_target"
	ReasonAmbiguousTarget = "ambiguous_target"
)

var (
	ErrRequired        = errors.New("logical session target field is required")
	ErrInvalidTarget   = errors.New("logical session target reference is invalid")
	ErrAmbiguousTarget = errors.New("logical session target reference is ambiguous")
)

type validationError struct {
	reason string
	field  string
	err    error
}

func (e *validationError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *validationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *validationError) Reason() string {
	if e == nil {
		return ""
	}
	return e.reason
}

func (e *validationError) Field() string {
	if e == nil {
		return ""
	}
	return e.field
}

// ValidationReasonFromError returns the validation reason and field when err is
// a logical-target validation error.
func ValidationReasonFromError(err error) (reason string, field string, ok bool) {
	var ve *validationError
	if !errors.As(err, &ve) || ve == nil {
		return "", "", false
	}
	return ve.reason, ve.field, true
}

func newValidationError(reason, field string, err error) error {
	if err == nil {
		return nil
	}
	return &validationError{
		reason: reason,
		field:  field,
		err:    err,
	}
}

func requiredFieldError(field, message string) error {
	if message == "" {
		message = fmt.Sprintf("%s is required", field)
	}
	return newValidationError(ReasonRequired, field, fmt.Errorf("%w: %s", ErrRequired, message))
}

func invalidTargetError(field, message string) error {
	if message == "" {
		message = "logical session target reference is invalid"
	}
	return newValidationError(ReasonInvalidTarget, field, fmt.Errorf("%w: %s", ErrInvalidTarget, message))
}

func ambiguousTargetError(field, message string) error {
	if message == "" {
		message = "logical session target reference is ambiguous"
	}
	return newValidationError(ReasonAmbiguousTarget, field, fmt.Errorf("%w: %s", ErrAmbiguousTarget, message))
}
