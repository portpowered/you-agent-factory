package service

import factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"

const factorySessionValidationTargetKind = "factory-session-validation"

const (
	factorySessionValidationReasonRequired       = "required"
	factorySessionValidationReasonMissing        = "missing"
	factorySessionValidationReasonNotDirectory   = "not_directory"
	factorySessionValidationReasonNotRunnable    = "not_runnable"
	factorySessionValidationReasonTargetNotFound = "target_not_found"
	factorySessionValidationReasonUnreadable     = "unreadable"
)

type factorySessionValidationError struct {
	reason string
	field  string
	err    error
}

func (e *factorySessionValidationError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *factorySessionValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *factorySessionValidationError) ErrorTargets() []factoryapi.ErrorTarget {
	if e == nil {
		return nil
	}
	return []factoryapi.ErrorTarget{factorySessionValidationErrorTarget(e.reason, e.field)}
}

func newFactorySessionValidationError(reason string, field string, err error) error {
	if err == nil {
		return nil
	}
	return &factorySessionValidationError{
		reason: reason,
		field:  field,
		err:    err,
	}
}

func factorySessionValidationErrorTarget(reason string, field string) factoryapi.ErrorTarget {
	target := factoryapi.ErrorTarget{Kind: factorySessionValidationTargetKind}
	if reason != "" {
		target.Id = &reason
	}
	if field != "" {
		target.Field = &field
	}
	return target
}
