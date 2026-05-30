package factorysessions

import (
	"errors"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

const validationTargetKind = "factory-session-validation"

const (
	validationReasonRequired       = "required"
	validationReasonMissing          = "missing"
	validationReasonNotDirectory     = "not_directory"
	validationReasonNotRunnable      = "not_runnable"
	validationReasonTargetNotFound   = "target_not_found"
	validationReasonUnreadable       = "unreadable"
)

// ValidationTargetKind is the API error-target kind for factory-session validation failures.
const ValidationTargetKind = validationTargetKind

// ValidationReasonRequired reports that a required session field was empty.
const ValidationReasonRequired = validationReasonRequired

// ValidationReasonMissing reports that the session folder path does not exist.
const ValidationReasonMissing = validationReasonMissing

// ValidationReasonNotDirectory reports that the session folder path is not a directory.
const ValidationReasonNotDirectory = validationReasonNotDirectory

// ValidationReasonNotRunnable reports that the folder exposes no runnable factory targets.
const ValidationReasonNotRunnable = validationReasonNotRunnable

// ValidationReasonTargetNotFound reports that the requested target was not discovered.
const ValidationReasonTargetNotFound = validationReasonTargetNotFound

// ValidationReasonUnreadable reports that the session folder could not be read.
const ValidationReasonUnreadable = validationReasonUnreadable

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

func (e *validationError) ErrorTargets() []factoryapi.ErrorTarget {
	if e == nil {
		return nil
	}
	return []factoryapi.ErrorTarget{validationErrorTarget(e.reason, e.field)}
}

// NewValidationError builds a structured factory-session validation error.
func NewValidationError(reason string, field string, err error) error {
	if err == nil {
		return nil
	}
	return &validationError{
		reason: reason,
		field:  field,
		err:    err,
	}
}

// ValidationReasonFromError returns the validation reason and field when err is a factory-session validation error.
func ValidationReasonFromError(err error) (reason string, field string, ok bool) {
	var ve *validationError
	if !errors.As(err, &ve) || ve == nil {
		return "", "", false
	}
	return ve.reason, ve.field, true
}

func validationErrorTarget(reason string, field string) factoryapi.ErrorTarget {
	target := factoryapi.ErrorTarget{Kind: validationTargetKind}
	if reason != "" {
		target.Id = &reason
	}
	if field != "" {
		target.Field = &field
	}
	return target
}
